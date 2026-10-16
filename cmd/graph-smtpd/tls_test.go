package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	netsmtp "net/smtp"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andrewheberle/configger"
	"github.com/spf13/pflag"
)

// writeCert writes a new self-signed certificate for localhost with the
// given common name to certFile and keyFile, replacing them by rename as a
// certificate renewal would, and returns the certificate.
func writeCert(t *testing.T, certFile, keyFile, commonName string) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("could not generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	if err != nil {
		t.Fatalf("could not generate serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("could not create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("could not parse certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("could not marshal key: %v", err)
	}

	// write the key first, so the watcher never pairs a new certificate with
	// an old key
	replaceFile(t, keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	replaceFile(t, certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))

	return cert
}

func replaceFile(t *testing.T, path string, data []byte) {
	t.Helper()

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		t.Fatalf("could not write %s: %v", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("could not replace %s: %v", path, err)
	}
}

// startTLSServer runs an SMTP server like graph-smtpd's, using newTLS for
// certFile and keyFile, and returns its address.
func startTLSServer(t *testing.T, certFile, keyFile string) string {
	t.Helper()

	sentinel, tlsConfig, err := newTLS(certFile, keyFile)
	if err != nil {
		t.Fatalf("newTLS() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = sentinel.Start(ctx) }()

	s := newServer(nopBackend{}, testConfig(t), slog.New(slog.DiscardHandler))
	s.TLSConfig = tlsConfig

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen: %v", err)
	}
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Close() })

	return l.Addr().String()
}

// testConfig returns graph-smtpd's configuration with default flag values.
func testConfig(t *testing.T) configger.Config {
	t.Helper()

	f := pflag.NewFlagSet("config", pflag.ContinueOnError)
	f.String("config", "", "")
	f.String("domain", "localhost", "")
	f.Int("recipients", 10, "")
	f.Int64("max", 1024*1024, "")
	f.Duration("conntimeout", 5*time.Minute, "")
	if err := f.Parse(nil); err != nil {
		t.Fatalf("could not parse flags: %v", err)
	}
	cfg, err := loadConfig(f)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	return cfg
}

// startTLS connects to addr, upgrades the connection with STARTTLS trusting
// only roots, and returns the client and the certificate the server sent.
func startTLS(t *testing.T, addr string, roots *x509.CertPool) (*netsmtp.Client, *x509.Certificate) {
	t.Helper()

	c, err := netsmtp.Dial(addr)
	if err != nil {
		t.Fatalf("could not dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.Hello("localhost"); err != nil {
		t.Fatalf("Hello() error = %v", err)
	}
	if ok, _ := c.Extension("STARTTLS"); !ok {
		t.Fatal("server does not offer STARTTLS")
	}
	if err := c.StartTLS(&tls.Config{ServerName: "localhost", RootCAs: roots}); err != nil {
		t.Fatalf("StartTLS() error = %v", err)
	}

	state, ok := c.TLSConnectionState()
	if !ok || len(state.PeerCertificates) == 0 {
		t.Fatal("connection is not using TLS")
	}

	return c, state.PeerCertificates[0]
}

func TestStartTLS(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	cert := writeCert(t, certFile, keyFile, "first")
	roots := x509.NewCertPool()
	roots.AddCert(cert)

	c, got := startTLS(t, startTLSServer(t, certFile, keyFile), roots)
	if got.Subject.CommonName != "first" {
		t.Errorf("server certificate = %q, want %q", got.Subject.CommonName, "first")
	}

	// the session carries on over the encrypted connection
	if err := c.Mail("alice@example.com"); err != nil {
		t.Fatalf("Mail() error = %v", err)
	}
	if err := c.Rcpt("bob@example.com"); err != nil {
		t.Fatalf("Rcpt() error = %v", err)
	}
	w, err := c.Data()
	if err != nil {
		t.Fatalf("Data() error = %v", err)
	}
	if _, err := w.Write([]byte("Subject: hello\r\n\r\nHello.\r\n")); err != nil {
		t.Fatalf("could not write message: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("message not accepted: %v", err)
	}
}

func TestStartTLSReloadsCertificate(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	roots := x509.NewCertPool()
	roots.AddCert(writeCert(t, certFile, keyFile, "first"))
	addr := startTLSServer(t, certFile, keyFile)

	if _, got := startTLS(t, addr, roots); got.Subject.CommonName != "first" {
		t.Fatalf("server certificate = %q, want %q", got.Subject.CommonName, "first")
	}

	// renew the certificate on disk; new connections should get it without
	// a restart
	roots.AddCert(writeCert(t, certFile, keyFile, "second"))

	deadline := time.Now().Add(5 * time.Second)
	for {
		_, got := startTLS(t, addr, roots)
		if got.Subject.CommonName == "second" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server certificate = %q after renewal, want %q", got.Subject.CommonName, "second")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestNoStartTLSWithoutCertificate(t *testing.T) {
	s := newServer(nopBackend{}, testConfig(t), slog.New(slog.DiscardHandler))

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not listen: %v", err)
	}
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Close() })

	c, err := netsmtp.Dial(l.Addr().String())
	if err != nil {
		t.Fatalf("could not dial: %v", err)
	}
	defer func() { _ = c.Close() }()

	if err := c.Hello("localhost"); err != nil {
		t.Fatalf("Hello() error = %v", err)
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		t.Error("server offers STARTTLS without a certificate")
	}
}
