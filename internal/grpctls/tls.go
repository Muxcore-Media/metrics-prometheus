package grpctls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	envInsecureDisable = "MUXCORE_INSECURE_DISABLE_TLS"
	envGRPCInsecure    = "MUXCORE_GRPC_INSECURE"

	envMetricsTLSCert = "METRICS_TLS_CERT"
	envMetricsTLSKey  = "METRICS_TLS_KEY"
	envMetricsTLSCA   = "METRICS_TLS_CA"
	envMetricsTLSDir  = "METRICS_TLS_DIR"

	envMuxcoreTLSCert = "MUXCORE_TLS_CERT"
	envMuxcoreTLSKey  = "MUXCORE_TLS_KEY"
	envMuxcoreTLSCA   = "MUXCORE_TLS_CA"

	defaultTLSDir = ".muxcore/tls/metrics-prometheus"
	moduleID      = "metrics-prometheus"

	caCertDuration     = 10 * 365 * 24 * time.Hour
	serverCertDuration = 365 * 24 * time.Hour
)

var serialLimit = new(big.Int).Lsh(big.NewInt(1), 128)

// InsecureAllowed reports whether inbound gRPC may run without TLS.
func InsecureAllowed() bool {
	return envTruthy(envInsecureDisable) || envTruthy(envGRPCInsecure)
}

// ServerConfig returns TLS settings for the module gRPC server, or nil when
// insecure mode is enabled.
func ServerConfig() (*tls.Config, error) {
	if InsecureAllowed() {
		return nil, nil
	}

	certFile := envFirst(envMetricsTLSCert, envMuxcoreTLSCert)
	keyFile := envFirst(envMetricsTLSKey, envMuxcoreTLSKey)
	caFile := envFirst(envMetricsTLSCA, envMuxcoreTLSCA)

	if certFile == "" || keyFile == "" {
		dir, err := tlsDir()
		if err != nil {
			return nil, err
		}
		certFile, keyFile, caFile, err = ensureAutoCerts(dir)
		if err != nil {
			return nil, err
		}
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("load gRPC TLS cert/key: %w", err)
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	if caFile != "" {
		caPEM, err := os.ReadFile(caFile) //nolint:gosec // path from env or generated dir
		if err != nil {
			return nil, fmt.Errorf("read gRPC TLS CA %q: %w", caFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("parse gRPC TLS CA %q", caFile)
		}
		cfg.ClientCAs = pool
		cfg.ClientAuth = tls.VerifyClientCertIfGiven
	}

	return cfg, nil
}

func envTruthy(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "true", "1":
		return true
	default:
		return false
	}
}

func envFirst(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func tlsDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv(envMetricsTLSDir)); v != "" {
		return expandHome(v), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve TLS dir: %w", err)
	}
	return filepath.Join(home, defaultTLSDir), nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func ensureAutoCerts(dir string) (certFile, keyFile, caFile string, err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", "", fmt.Errorf("create TLS dir %q: %w", dir, err)
	}

	caCertPath := filepath.Join(dir, "ca.crt")
	caKeyPath := filepath.Join(dir, "ca.key")
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")

	if filesExist(caCertPath, caKeyPath, certPath, keyPath) {
		return certPath, keyPath, caCertPath, nil
	}

	caCert, caKey, caPEM, err := generateCA()
	if err != nil {
		return "", "", "", err
	}
	if err := writePEM(caCertPath, caPEM, 0o600); err != nil {
		return "", "", "", err
	}
	if err := writeKey(caKeyPath, caKey); err != nil {
		return "", "", "", err
	}

	certPEM, keyPEM, err := issueServerCert(caCert, caKey)
	if err != nil {
		return "", "", "", err
	}
	if err := writePEM(certPath, certPEM, 0o600); err != nil {
		return "", "", "", err
	}
	if err := writePEM(keyPath, keyPEM, 0o600); err != nil {
		return "", "", "", err
	}

	return certPath, keyPath, caCertPath, nil
}

func filesExist(paths ...string) bool {
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			return false
		}
	}
	return true
}

func generateCA() (*x509.Certificate, *ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("generate CA key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "MuxCore " + moduleID + " CA",
			Organization: []string{"MuxCore"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(caCertDuration),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create CA cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, nil, err
	}

	return cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), nil
}

func issueServerCert(caCert *x509.Certificate, caKey *ecdsa.PrivateKey) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate server key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return nil, nil, err
	}

	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   moduleID,
			Organization: []string{"MuxCore"},
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(serverCertDuration),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:    []string{"localhost", moduleID},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create server cert: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal server key: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}),
		nil
}

func writePEM(path string, data []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}

func writeKey(path string, key *ecdsa.PrivateKey) error {
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	return writePEM(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600)
}
