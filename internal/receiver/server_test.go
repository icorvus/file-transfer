package receiver

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	s, err := New(Config{Name: "Test PC", OutputDir: filepath.Join(root, "photos"), StateDir: filepath.Join(root, "state"), Port: 8787, AdminPort: 8788, MaxBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func pairPhone(t *testing.T, s *Server) string {
	t.Helper()
	code, _ := randomHex(32)
	s.mu.Lock()
	s.pairCode = code
	s.pairExpiry = time.Now().Add(time.Minute)
	s.mu.Unlock()
	body, _ := json.Marshal(map[string]string{"code": code, "name": "Test iPhone"})
	r := httptest.NewRequest("POST", "/v1/pair", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.TransferHandler().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("pair %d: %s", w.Code, w.Body.String())
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Token) != 64 {
		t.Fatal("missing token")
	}
	return response.Token
}

func uploadRequest(body []byte, token, id, name string) *http.Request {
	r := httptest.NewRequest("POST", "/v1/files", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Transfer-ID", id)
	r.Header.Set("X-Filename-Base64", base64.StdEncoding.EncodeToString([]byte(name)))
	r.Header.Set("X-Content-SHA256", digest(body))
	return r
}

func send(s *Server, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.TransferHandler().ServeHTTP(w, r)
	return w
}

func TestPairingSingleUseExpiryPersistenceAndRevocation(t *testing.T) {
	s := newTestServer(t)
	code, _ := randomHex(32)
	s.pairCode = code
	s.pairExpiry = time.Now().Add(-time.Second)
	body := fmt.Sprintf(`{"name":"iPhone","code":%q}`, code)
	if w := send(s, httptest.NewRequest("POST", "/v1/pair", strings.NewReader(body))); w.Code != 403 {
		t.Fatal(w.Code)
	}
	s.pairExpiry = time.Now().Add(time.Minute)
	if w := send(s, httptest.NewRequest("POST", "/v1/pair", strings.NewReader(body))); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := send(s, httptest.NewRequest("POST", "/v1/pair", strings.NewReader(body))); w.Code != 403 {
		t.Fatal("reused code accepted")
	}
	token := pairPhone(t, s)
	restarted, err := New(s.Config)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Fingerprint != s.Fingerprint || restarted.ID() != s.ID() {
		t.Fatal("identity changed after restart")
	}
	r := httptest.NewRequest("GET", "/v1/status", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	if w := send(restarted, r); w.Code != 200 {
		t.Fatal("pairing lost after restart", w.Body.String())
	}
	device, _ := restarted.authenticated(r)
	revoke := httptest.NewRequest("POST", "http://127.0.0.1:8788/api/revoke", strings.NewReader(fmt.Sprintf(`{"id":%q}`, device.ID)))
	revoke.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	restarted.AdminHandler().ServeHTTP(w, revoke)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := send(restarted, r); w.Code != 401 {
		t.Fatal("revoked phone still accepted")
	}
	data, _ := os.ReadFile(filepath.Join(s.Config.StateDir, "identity.json"))
	if bytes.Contains(data, []byte(token)) {
		t.Fatal("raw bearer token persisted")
	}
}

func TestTransferOriginalBytesCollisionAndPersistentRetry(t *testing.T) {
	s := newTestServer(t)
	token := pairPhone(t, s)
	body := append([]byte("HEIC test bytes\x00\xff"), bytes.Repeat([]byte{42}, 8192)...)
	id := "01234567-0123-4567-8901-012345678901"
	w := send(s, uploadRequest(body, token, id, "Zdjęcie.HEIC"))
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var receipt Receipt
	if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(filepath.Join(s.Config.OutputDir, receipt.Name))
	if err != nil || !bytes.Equal(body, actual) {
		t.Fatal("file bytes changed", err)
	}
	if receipt.SHA256 != digest(body) || receipt.Bytes != int64(len(body)) || !receipt.Committed {
		t.Fatal("bad receipt", receipt)
	}
	restarted, err := New(s.Config)
	if err != nil {
		t.Fatal(err)
	}
	if w := send(restarted, uploadRequest(body, token, id, "Zdjęcie.HEIC")); w.Code != 200 {
		t.Fatal("retry not deduplicated", w.Code, w.Body.String())
	}
	if w := send(restarted, uploadRequest(body, token, "11234567-0123-4567-8901-012345678901", "Zdjęcie.HEIC")); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.Config.OutputDir, "Zdjęcie (2).HEIC")); err != nil {
		t.Fatal("collision not renamed", err)
	}
	if w := send(s, uploadRequest([]byte("different"), token, id, "Zdjęcie.HEIC")); w.Code != 409 {
		t.Fatal("conflicting transfer ID accepted", w.Code)
	}
	files, _ := os.ReadDir(s.Config.OutputDir)
	if len(files) != 2 {
		t.Fatal("duplicate or partial files", len(files))
	}
}

func TestUploadValidationLeavesNoFiles(t *testing.T) {
	cases := []struct {
		name   string
		change func(*http.Request)
		want   int
	}{
		{"unauthorized", func(r *http.Request) { r.Header.Del("Authorization") }, 401},
		{"bad hash", func(r *http.Request) { r.Header.Set("X-Content-SHA256", strings.Repeat("a", 64)) }, 422},
		{"oversized", func(r *http.Request) { r.ContentLength = 2 << 20 }, 413},
		{"missing length", func(r *http.Request) { r.ContentLength = -1 }, 411},
		{"interrupted", func(r *http.Request) { r.ContentLength = 500 }, 400},
		{"bad ID", func(r *http.Request) { r.Header.Set("X-Transfer-ID", "../../outside") }, 400},
		{"bad filename", func(r *http.Request) { r.Header.Set("X-Filename-Base64", "invalid!") }, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			token := pairPhone(t, s)
			r := uploadRequest([]byte("photo"), token, "01234567-0123-4567-8901-012345678901", "x.jpg")
			tc.change(r)
			w := send(s, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
			files, _ := os.ReadDir(s.Config.OutputDir)
			if len(files) != 0 {
				t.Fatal("partial file leaked")
			}
		})
	}
}

func TestUnsafeFilenamesAndSymlinksNeverOverwrite(t *testing.T) {
	s := newTestServer(t)
	token := pairPhone(t, s)
	outside := filepath.Join(t.TempDir(), "original")
	if err := os.WriteFile(outside, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.Config.OutputDir, "photo.jpg")); err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"../../outside.jpg", "photo.jpg", "a\\..\\b.jpg", strings.Repeat("ż", 150) + ".heic"} {
		w := send(s, uploadRequest([]byte("photo"), token, fmt.Sprintf("01234567-0123-4567-8901-%012d", i), name))
		if w.Code != 201 {
			t.Fatal(w.Code, w.Body.String())
		}
		var receipt Receipt
		_ = json.Unmarshal(w.Body.Bytes(), &receipt)
		if filepath.Base(receipt.Name) != receipt.Name || strings.HasPrefix(receipt.Name, ".") {
			t.Fatal("unsafe name", receipt.Name)
		}
	}
	b, _ := os.ReadFile(outside)
	if string(b) != "keep" {
		t.Fatal("symlink target overwritten")
	}
}

func TestConcurrentUploadsAndIdempotency(t *testing.T) {
	s := newTestServer(t)
	token := pairPhone(t, s)
	var group sync.WaitGroup
	for i := 0; i < 3; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			w := send(s, uploadRequest(bytes.Repeat([]byte{7}, 65536), token, "01234567-0123-4567-8901-012345678901", "same.jpg"))
			if w.Code != 200 && w.Code != 201 {
				t.Error(w.Code, w.Body.String())
			}
		}()
	}
	group.Wait()
	files, _ := os.ReadDir(s.Config.OutputDir)
	if len(files) != 1 {
		t.Fatal("concurrent retry duplicated file", len(files))
	}
}

func TestPreparedReceiptRecoversAfterCrash(t *testing.T) {
	s := newTestServer(t)
	token := pairPhone(t, s)
	id := "01234567-0123-4567-8901-012345678901"
	body := []byte("photo")
	w := send(s, uploadRequest(body, token, id, "photo.heic"))
	if w.Code != 201 {
		t.Fatal(w.Code)
	}
	var receipt Receipt
	_ = json.Unmarshal(w.Body.Bytes(), &receipt)
	receipt.Committed = false
	path := s.receiptPath(s.state.Devices[0].ID, id)
	if err := atomicJSON(path, receipt); err != nil {
		t.Fatal(err)
	}
	if w := send(s, uploadRequest(body, token, id, "photo.heic")); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	files, _ := os.ReadDir(s.Config.OutputDir)
	if len(files) != 1 {
		t.Fatal("recovery duplicated file")
	}
}

func TestAdminRejectsCrossOriginAndDNSRebinding(t *testing.T) {
	s := newTestServer(t)
	for _, tc := range []struct {
		host, origin, content, method string
		want                          int
	}{
		{"evil.example:8788", "", "", "GET", 403},
		{"127.0.0.1:8788", "https://evil.example", "application/json", "POST", 403},
		{"127.0.0.1:8788", "", "text/plain", "POST", 415},
		{"127.0.0.1:8788", "", "", "GET", 200},
	} {
		r := httptest.NewRequest(tc.method, "http://"+tc.host+"/api/status", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", tc.content)
		w := httptest.NewRecorder()
		s.AdminHandler().ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatal(tc, w.Code)
		}
	}
}

func TestRealTLSRoundTrip(t *testing.T) {
	s := newTestServer(t)
	token := pairPhone(t, s)
	ts := httptest.NewUnstartedServer(s.TransferHandler())
	ts.TLS = &tls.Config{Certificates: []tls.Certificate{s.Certificate}, MinVersion: tls.VersionTLS12}
	ts.StartTLS()
	defer ts.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, VerifyConnection: func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 || digest(cs.PeerCertificates[0].Raw) != s.Fingerprint {
			return fmt.Errorf("certificate pin mismatch")
		}
		return nil
	}}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	body := []byte("real HTTPS transfer")
	r := uploadRequest(body, token, "01234567-0123-4567-8901-012345678901", "photo.jpg")
	r.URL.Scheme = "https"
	r.URL.Host = strings.TrimPrefix(ts.URL, "https://")
	r.RequestURI = ""
	response, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		data, _ := io.ReadAll(response.Body)
		t.Fatal(response.StatusCode, string(data))
	}
}
