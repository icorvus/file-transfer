package receiver

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TokenHash string    `json:"tokenHash,omitempty"`
	PairedAt  time.Time `json:"pairedAt"`
}

type diskState struct {
	ID          string   `json:"id"`
	Certificate string   `json:"certificate"`
	PrivateKey  string   `json:"privateKey"`
	Devices     []Device `json:"devices"`
}

func randomHex(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func digest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// atomicJSON syncs both the file and directory before acknowledging persisted state.
func atomicJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func loadState(dir string) (diskState, tls.Certificate, string, error) {
	var state diskState
	if err := os.MkdirAll(dir, 0700); err != nil {
		return state, tls.Certificate{}, "", err
	}
	path := filepath.Join(dir, "identity.json")
	data, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(data, &state)
	} else if os.IsNotExist(err) {
		state, err = newIdentity()
		if err == nil {
			err = atomicJSON(path, state)
		}
	}
	if err != nil {
		return state, tls.Certificate{}, "", err
	}
	cert, err := tls.X509KeyPair([]byte(state.Certificate), []byte(state.PrivateKey))
	if err != nil {
		return state, cert, "", fmt.Errorf("read receiver identity: %w", err)
	}
	if state.ID == "" || len(cert.Certificate) == 0 {
		return state, cert, "", fmt.Errorf("invalid receiver identity")
	}
	return state, cert, digest(cert.Certificate[0]), nil
}

func newIdentity() (diskState, error) {
	var s diskState
	id, err := randomHex(16)
	if err != nil {
		return s, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return s, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return s, err
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "NearDrop " + id},
		NotBefore: time.Now().Add(-24 * time.Hour), NotAfter: time.Now().AddDate(20, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{"neardrop.local"}, BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return s, err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return s, err
	}
	s.ID = id
	s.Certificate = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	s.PrivateKey = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
	s.Devices = []Device{}
	return s, nil
}
