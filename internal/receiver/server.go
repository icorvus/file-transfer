package receiver

import (
	"crypto/subtle"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

//go:embed web/*
var web embed.FS

type Config struct {
	Name      string
	OutputDir string
	StateDir  string
	Port      int
	AdminPort int
	MaxBytes  int64
}

type Server struct {
	Config      Config
	Certificate tls.Certificate
	Fingerprint string
	mu          sync.Mutex
	state       diskState
	pairCode    string
	pairExpiry  time.Time
	recent      []Receipt
	uploadSlots chan struct{}
}

func New(c Config) (*Server, error) {
	if c.MaxBytes <= 0 {
		return nil, fmt.Errorf("max file size must be positive")
	}
	var err error
	c.OutputDir, err = filepath.Abs(c.OutputDir)
	if err != nil {
		return nil, err
	}
	c.StateDir, err = filepath.Abs(c.StateDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(c.OutputDir, 0700); err != nil {
		return nil, err
	}
	state, cert, fp, err := loadState(c.StateDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Join(c.StateDir, "receipts"), 0700); err != nil {
		return nil, err
	}
	return &Server{Config: c, Certificate: cert, Fingerprint: fp, state: state, recent: []Receipt{}, uploadSlots: make(chan struct{}, 3)}, nil
}

func (s *Server) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.ID
}

func (s *Server) TransferHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("POST /v1/pair", s.pair)
	m.HandleFunc("GET /v1/status", s.status)
	m.HandleFunc("POST /v1/files", s.upload)
	return headers(m)
}

func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func respond(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}

func fail(w http.ResponseWriter, code int, message string) {
	respond(w, code, map[string]string{"error": message})
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(into); err != nil {
		fail(w, 400, "Invalid request")
		return false
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		fail(w, 400, "Expected one JSON object")
		return false
	}
	return true
}

func (s *Server) authenticated(r *http.Request) (Device, bool) {
	if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return Device{}, false
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if len(token) != 64 {
		return Device{}, false
	}
	hash := digest([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.state.Devices {
		if subtle.ConstantTimeCompare([]byte(d.TokenHash), []byte(hash)) == 1 {
			return d, true
		}
	}
	return Device{}, false
}

func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 120 {
		fail(w, 400, "A device name is required (up to 120 bytes)")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pairCode) != 64 || time.Now().After(s.pairExpiry) || subtle.ConstantTimeCompare([]byte(req.Code), []byte(s.pairCode)) != 1 {
		fail(w, 403, "Pairing code expired or already used. Create a new QR code on the PC.")
		return
	}
	token, err := randomHex(32)
	if err != nil {
		fail(w, 500, "Could not create device credentials")
		return
	}
	id, err := randomHex(16)
	if err != nil {
		fail(w, 500, "Could not create device identity")
		return
	}
	device := Device{ID: id, Name: name, TokenHash: digest([]byte(token)), PairedAt: time.Now().UTC()}
	next := s.state
	next.Devices = append(append([]Device{}, s.state.Devices...), device)
	if err := atomicJSON(filepath.Join(s.Config.StateDir, "identity.json"), next); err != nil {
		slog.Error("persist pairing", "error", err)
		fail(w, 500, "Could not save pairing on PC")
		return
	}
	s.state = next
	s.pairCode = ""
	respond(w, 201, map[string]any{"id": s.state.ID, "name": s.Config.Name, "deviceId": id, "token": token, "maxBytes": s.Config.MaxBytes})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticated(r); !ok {
		fail(w, 401, "Pair this iPhone with the PC again")
		return
	}
	respond(w, 200, map[string]any{"id": s.ID(), "name": s.Config.Name, "maxBytes": s.Config.MaxBytes})
}

// AdminHandler must be bound only to loopback. Host and Origin checks also
// protect against DNS rebinding and requests made by an unrelated website.
func (s *Server) AdminHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/status", s.adminStatus)
	m.HandleFunc("POST /api/pairing", s.adminPairing)
	m.HandleFunc("POST /api/revoke", s.adminRevoke)
	m.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if name != "index.html" && name != "app.js" && name != "style.css" {
			http.NotFound(w, r)
			return
		}
		b, err := web.ReadFile("web/" + name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		contentTypes := map[string]string{"index.html": "text/html; charset=utf-8", "app.js": "text/javascript; charset=utf-8", "style.css": "text/css; charset=utf-8"}
		w.Header().Set("Content-Type", contentTypes[name])
		_, _ = w.Write(b)
	})
	return headers(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, port, err := net.SplitHostPort(r.Host)
		if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") || port != fmt.Sprint(s.Config.AdminPort) {
			fail(w, 403, "Open this page on the PC using localhost")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			fail(w, 403, "Origin not allowed")
			return
		}
		if r.Method != "GET" && r.Header.Get("Content-Type") != "application/json" {
			fail(w, 415, "Expected application/json")
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		m.ServeHTTP(w, r)
	}))
}

func (s *Server) adminStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	devices := append([]Device{}, s.state.Devices...)
	for i := range devices {
		devices[i].TokenHash = ""
	}
	recent := append([]Receipt{}, s.recent...)
	s.mu.Unlock()
	respond(w, 200, map[string]any{"name": s.Config.Name, "id": s.ID(), "outputDir": s.Config.OutputDir, "devices": devices, "recent": recent, "addresses": LANAddresses(), "port": s.Config.Port})
}

func LANAddresses() []string {
	result := []string{}
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagRunning == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.To4() != nil && ip.IsPrivate() {
				result = append(result, ip.String())
			}
		}
	}
	return result
}

func (s *Server) adminPairing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Host string `json:"host"`
	}
	if !decode(w, r, &req) {
		return
	}
	allowed := false
	for _, address := range LANAddresses() {
		if address == req.Host {
			allowed = true
		}
	}
	if !allowed {
		fail(w, 400, "Choose this PC's local network address")
		return
	}
	code, err := randomHex(32)
	if err != nil {
		fail(w, 500, "Could not generate pairing code")
		return
	}
	expires := time.Now().Add(10 * time.Minute)
	s.mu.Lock()
	s.pairCode = code
	s.pairExpiry = expires
	s.mu.Unlock()
	q := url.Values{"v": {"1"}, "host": {req.Host}, "port": {fmt.Sprint(s.Config.Port)}, "id": {s.ID()}, "name": {s.Config.Name}, "fp": {s.Fingerprint}, "code": {code}}
	uri := "neardrop://pair?" + q.Encode()
	png, err := qrcode.Encode(uri, qrcode.Medium, 384)
	if err != nil {
		fail(w, 500, "Could not generate QR code")
		return
	}
	respond(w, 200, map[string]any{"uri": uri, "qr": png, "expiresAt": expires})
}

func (s *Server) adminRevoke(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &req) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.state
	next.Devices = []Device{}
	for _, d := range s.state.Devices {
		if d.ID != req.ID {
			next.Devices = append(next.Devices, d)
		}
	}
	if err := atomicJSON(filepath.Join(s.Config.StateDir, "identity.json"), next); err != nil {
		fail(w, 500, "Could not remove pairing")
		return
	}
	s.state = next
	respond(w, 200, map[string]bool{"ok": true})
}
