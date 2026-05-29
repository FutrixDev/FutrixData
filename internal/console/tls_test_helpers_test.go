package console

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"
)

func makeTestCertificatePEM(t *testing.T) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "futrixdata-test-ca"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	block := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	})
	if len(block) == 0 {
		t.Fatalf("encode pem: empty")
	}
	return string(block)
}

func writeTestCertificateFile(t *testing.T, certPEM string) string {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "root-ca-*.pem")
	if err != nil {
		t.Fatalf("create temp cert file: %v", err)
	}
	path := file.Name()

	if _, err := file.WriteString(strings.TrimSpace(certPEM) + "\n"); err != nil {
		_ = file.Close()
		t.Fatalf("write temp cert file: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close temp cert file: %v", err)
	}

	return path
}
