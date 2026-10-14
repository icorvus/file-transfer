package receiver

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Receipt struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	OriginalName string    `json:"originalName"`
	SHA256       string    `json:"sha256"`
	Bytes        int64     `json:"bytes"`
	SavedAt      time.Time `json:"savedAt"`
	DeviceName   string    `json:"deviceName"`
	Committed    bool      `json:"committed"`
}

var transferID = regexp.MustCompile(`^[a-zA-Z0-9-]{16,64}$`)
var shaPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func cleanName(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", fmt.Errorf("invalid UTF-8 filename")
	}
	name := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) || r == ':' {
			return '_'
		}
		return r
	}, raw)
	name = strings.Trim(name, " .")
	if name == "" {
		return "", fmt.Errorf("empty filename")
	}
	if len(name) > 180 {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		stem := strings.TrimSuffix(name, ext)
		for len(stem) > 180-len(ext) {
			_, n := utf8.DecodeLastRuneInString(stem)
			stem = stem[:len(stem)-n]
		}
		name = stem + ext
	}
	return name, nil
}

func (s *Server) receiptPath(deviceID, id string) string {
	return filepath.Join(s.Config.StateDir, "receipts", deviceID+"-"+id+".json")
}

// Called with the server mutex held. A prepared receipt recovers a crash between
// publishing a complete file and committing its acknowledgement.
func (s *Server) existingReceipt(path, hash, original string, size int64) (*Receipt, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var receipt Receipt
	if err := json.Unmarshal(b, &receipt); err != nil {
		return nil, err
	}
	if receipt.SHA256 != hash || receipt.Bytes != size || receipt.OriginalName != original {
		return nil, errConflict
	}
	if receipt.Committed {
		return &receipt, nil
	}
	file, err := os.Open(filepath.Join(s.Config.OutputDir, receipt.Name))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	h := sha256.New()
	n, err := io.Copy(h, file)
	if err != nil {
		return nil, err
	}
	if n != receipt.Bytes || hex.EncodeToString(h.Sum(nil)) != receipt.SHA256 {
		return nil, nil
	}
	if err := syncDir(s.Config.OutputDir); err != nil {
		return nil, err
	}
	receipt.Committed = true
	if err := atomicJSON(path, receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

var errConflict = errors.New("transfer ID already used for different content")

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	device, ok := s.authenticated(r)
	if !ok {
		fail(w, 401, "Pair this iPhone with the PC again")
		return
	}
	id := r.Header.Get("X-Transfer-ID")
	hash := r.Header.Get("X-Content-SHA256")
	decoded, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Filename-Base64"))
	name, nameErr := cleanName(string(decoded))
	if !transferID.MatchString(id) || !shaPattern.MatchString(hash) || err != nil || nameErr != nil {
		fail(w, 400, "Invalid transfer ID, checksum, or filename")
		return
	}
	if r.ContentLength <= 0 {
		fail(w, 411, "A nonempty file and Content-Length are required")
		return
	}
	if r.ContentLength > s.Config.MaxBytes {
		fail(w, 413, "File exceeds the receiver's size limit")
		return
	}
	select {
	case s.uploadSlots <- struct{}{}:
		defer func() { <-s.uploadSlots }()
	default:
		w.Header().Set("Retry-After", "2")
		fail(w, 503, "Receiver busy. Try again shortly.")
		return
	}
	path := s.receiptPath(device.ID, id)
	s.mu.Lock()
	existing, err := s.existingReceipt(path, hash, name, r.ContentLength)
	s.mu.Unlock()
	if errors.Is(err, errConflict) {
		fail(w, 409, err.Error())
		return
	}
	if err != nil {
		slog.Error("read receipt", "error", err)
		fail(w, 500, "Could not check transfer receipt")
		return
	}
	if existing != nil {
		respond(w, 200, existing)
		return
	}

	f, err := os.CreateTemp(s.Config.OutputDir, ".neardrop-partial-*")
	if err != nil {
		fail(w, 507, "Cannot create a file in the destination folder")
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	h := sha256.New()
	r.Body = http.MaxBytesReader(w, r.Body, s.Config.MaxBytes)
	n, err := io.Copy(io.MultiWriter(f, h), r.Body)
	if err != nil || n != r.ContentLength {
		fail(w, 400, "Transfer interrupted; retry to send the complete file")
		return
	}
	if hex.EncodeToString(h.Sum(nil)) != hash {
		fail(w, 422, "File checksum did not match; retry the transfer")
		return
	}
	if err = f.Sync(); err != nil {
		fail(w, 507, "Could not save file to disk")
		return
	}
	if err = f.Close(); err != nil {
		fail(w, 507, "Could not close saved file")
		return
	}
	if r.Context().Err() != nil {
		return
	}
	// Serialize only the commit stage. Concurrent uploads cannot overwrite one
	// another, and duplicate requests for the same ID produce a single file.
	s.mu.Lock()
	defer s.mu.Unlock()
	stillPaired := false
	for _, d := range s.state.Devices {
		if d.ID == device.ID {
			stillPaired = true
		}
	}
	if !stillPaired {
		fail(w, 401, "This iPhone was unpaired during the transfer")
		return
	}
	existing, err = s.existingReceipt(path, hash, name, n)
	if errors.Is(err, errConflict) {
		fail(w, 409, err.Error())
		return
	}
	if err != nil {
		fail(w, 500, "Could not check transfer receipt")
		return
	}
	if existing != nil {
		respond(w, 200, existing)
		return
	}
	receipt := Receipt{ID: id, OriginalName: name, SHA256: hash, Bytes: n, SavedAt: time.Now().UTC(), DeviceName: device.Name}
	stem, ext := strings.TrimSuffix(name, filepath.Ext(name)), filepath.Ext(name)
	for suffix := 1; suffix <= 10000; suffix++ {
		receipt.Name = name
		if suffix > 1 {
			receipt.Name = fmt.Sprintf("%s (%d)%s", stem, suffix, ext)
		}
		destination := filepath.Join(s.Config.OutputDir, receipt.Name)
		if _, err := os.Lstat(destination); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			fail(w, 507, "Cannot access destination folder")
			return
		}
		if err = atomicJSON(path, receipt); err != nil {
			fail(w, 507, "Could not persist transfer receipt")
			return
		}
		// Hard link publishes a complete file atomically without replacing any
		// existing file, including a symlink created between Lstat and Link.
		if err = os.Link(f.Name(), destination); os.IsExist(err) {
			continue
		}
		if err != nil {
			slog.Error("publish file", "error", err)
			fail(w, 507, "Could not publish file; destination must support hard links")
			return
		}
		if err = syncDir(s.Config.OutputDir); err != nil {
			fail(w, 507, "Could not sync destination folder")
			return
		}
		receipt.Committed = true
		if err = atomicJSON(path, receipt); err != nil {
			fail(w, 507, "File saved but receipt pending; retry safely")
			return
		}
		s.recent = append([]Receipt{receipt}, s.recent...)
		if len(s.recent) > 30 {
			s.recent = s.recent[:30]
		}
		slog.Info("photo saved", "name", receipt.Name, "bytes", n, "device", device.Name)
		respond(w, 201, receipt)
		return
	}
	fail(w, 409, "Too many files with this name")
}
