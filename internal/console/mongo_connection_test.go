package console

import (
	"strings"
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestMongoURI_HostModeIncludesTLSWhenSSLEnabled(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypeMongoDB,
		Host: "127.0.0.1",
		Port: 27017,
		Options: map[string]any{
			"tls":        false,
			"sslEnabled": true,
		},
	}

	uri, err := mongoURI(ds)
	if err != nil {
		t.Fatalf("mongoURI: %v", err)
	}
	if !strings.Contains(strings.ToLower(uri), "tls=true") {
		t.Fatalf("expected tls=true when sslEnabled=true, got %q", uri)
	}
}

func TestMongoURI_HostModeDisablesTLSWhenSSLEnabledFalse(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypeMongoDB,
		Host: "127.0.0.1",
		Port: 27017,
		Options: map[string]any{
			"tls":        true,
			"sslEnabled": false,
		},
	}

	uri, err := mongoURI(ds)
	if err != nil {
		t.Fatalf("mongoURI: %v", err)
	}
	if strings.Contains(strings.ToLower(uri), "tls=true") {
		t.Fatalf("expected tls=false when sslEnabled=false, got %q", uri)
	}
}

func TestMongoURI_DirectURIEnablesTLSWhenCertificateProvided(t *testing.T) {
	ds := datasource.DataSource{
		Type: datasource.TypeMongoDB,
		Options: map[string]any{
			"uri":         "mongodb://127.0.0.1:27017/admin",
			"sslrootcert": makeTestCertificatePEM(t),
		},
	}

	uri, err := mongoURI(ds)
	if err != nil {
		t.Fatalf("mongoURI: %v", err)
	}
	if !strings.Contains(strings.ToLower(uri), "tls=true") {
		t.Fatalf("expected tls=true when sslrootcert is present, got %q", uri)
	}
}

func TestMongoTLSConfig_UsesCertificatePathWhenSSLEnabled(t *testing.T) {
	certPath := writeTestCertificateFile(t, makeTestCertificatePEM(t))
	ds := datasource.DataSource{
		Type: datasource.TypeMongoDB,
		Options: map[string]any{
			"sslEnabled":  true,
			"sslrootcert": certPath,
		},
	}

	tlsConfig, err := mongoTLSConfig(ds)
	if err != nil {
		t.Fatalf("mongoTLSConfig: %v", err)
	}
	if tlsConfig == nil {
		t.Fatalf("expected tls config when certificate path is provided")
	}
	if tlsConfig.RootCAs == nil {
		t.Fatalf("expected RootCAs to be set")
	}
}

func TestMongoKey_ChangesWhenCertificateChanges(t *testing.T) {
	dsA := datasource.DataSource{
		ID:   "mongo_ssl",
		Type: datasource.TypeMongoDB,
		Options: map[string]any{
			"tls":         true,
			"sslEnabled":  true,
			"sslrootcert": makeTestCertificatePEM(t),
		},
	}
	dsB := datasource.DataSource{
		ID:   "mongo_ssl",
		Type: datasource.TypeMongoDB,
		Options: map[string]any{
			"tls":         true,
			"sslEnabled":  true,
			"sslrootcert": makeTestCertificatePEM(t),
		},
	}

	keyA := mongoKey(dsA)
	keyB := mongoKey(dsB)
	if keyA == keyB {
		t.Fatalf("expected mongo key to change when ssl certificate changes, got key %q", keyA)
	}
}
