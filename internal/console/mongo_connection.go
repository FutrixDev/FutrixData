package console

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"futrixdata/platform/internal/datasource"
)

func (m *MongoAdapter) clientFor(ds datasource.DataSource) (*mongo.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := mongoKey(ds)
	if prev, ok := m.byID[ds.ID]; ok && prev != key {
		if old, ok := m.clients[prev]; ok {
			_ = old.Disconnect(context.Background())
			delete(m.clients, prev)
		}
	}
	if client, ok := m.clients[key]; ok {
		return client, nil
	}

	uri, err := mongoURI(ds)
	if err != nil {
		return nil, err
	}
	opts := options.Client().ApplyURI(uri).SetServerSelectionTimeout(5 * time.Second)
	tlsConfig, err := mongoTLSConfig(ds)
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		opts = opts.SetTLSConfig(tlsConfig)
	}
	client, err := mongo.Connect(context.Background(), opts)
	if err != nil {
		return nil, err
	}
	m.clients[key] = client
	m.byID[ds.ID] = key
	return client, nil
}

func mongoURI(ds datasource.DataSource) (string, error) {
	// Priority 1: Direct URI from options
	if ds.Options != nil {
		if uri, ok := ds.Options["uri"].(string); ok && strings.TrimSpace(uri) != "" {
			return ensureMongoTLSParam(uri, mongoTLSEnabled(ds)), nil
		}
	}

	// Priority 2: Replica Set mode with multiple hosts
	if ds.Options != nil {
		if hostsRaw, ok := ds.Options["hosts"]; ok {
			return buildReplicaSetURI(ds, hostsRaw)
		}
	}

	// Priority 3: Single host mode (legacy)
	return buildSingleHostURI(ds)
}

func buildReplicaSetURI(ds datasource.DataSource, hostsRaw any) (string, error) {
	var hosts []string
	switch v := hostsRaw.(type) {
	case []string:
		hosts = v
	case []any:
		for _, h := range v {
			if s, ok := h.(string); ok && strings.TrimSpace(s) != "" {
				hosts = append(hosts, s)
			}
		}
	}
	if len(hosts) == 0 {
		return "", errors.New("hosts array is empty or invalid")
	}

	var auth string
	if ds.Username != "" {
		auth = url.UserPassword(ds.Username, ds.Password).String() + "@"
	}

	uri := fmt.Sprintf("mongodb://%s%s", auth, strings.Join(hosts, ","))

	params := url.Values{}
	if ds.AuthSource != "" {
		params.Set("authSource", ds.AuthSource)
	} else if ds.Database != "" {
		params.Set("authSource", ds.Database)
	}
	if mongoTLSEnabled(ds) {
		params.Set("tls", "true")
	}
	if ds.Options != nil {
		if replicaSet, ok := ds.Options["replicaSet"].(string); ok && replicaSet != "" {
			params.Set("replicaSet", replicaSet)
		}
	}
	if len(params) > 0 {
		uri = uri + "/?" + params.Encode()
	}
	return uri, nil
}

func buildSingleHostURI(ds datasource.DataSource) (string, error) {
	if strings.TrimSpace(ds.Host) == "" || ds.Port == 0 {
		return "", errors.New("host and port are required")
	}
	host := fmt.Sprintf("%s:%d", ds.Host, ds.Port)
	var auth string
	if ds.Username != "" {
		auth = url.UserPassword(ds.Username, ds.Password).String() + "@"
	}
	uri := fmt.Sprintf("mongodb://%s%s", auth, host)
	params := url.Values{}
	if ds.AuthSource != "" {
		params.Set("authSource", ds.AuthSource)
	} else if ds.Database != "" {
		params.Set("authSource", ds.Database)
	}
	if mongoTLSEnabled(ds) {
		params.Set("tls", "true")
	}
	if len(params) > 0 {
		uri = uri + "/?" + params.Encode()
	}
	return uri, nil
}

func mongoTLSEnabled(ds datasource.DataSource) bool {
	if ds.Options == nil {
		return false
	}
	sslEnabled, hasSSLEnabled := optionBool(ds.Options, "sslEnabled")
	if hasSSLEnabled {
		return sslEnabled
	}
	enabled, hasTLS := optionBool(ds.Options, "tls")
	if hasTLS {
		return enabled
	}
	return strings.TrimSpace(optionString(ds.Options, "sslrootcert")) != ""
}

func mongoTLSConfig(ds datasource.DataSource) (*tls.Config, error) {
	if !mongoTLSEnabled(ds) {
		return nil, nil
	}
	certPEM, err := mongoRootCertificatePEM(ds.Options)
	if err != nil {
		return nil, err
	}
	if len(certPEM) == 0 {
		return nil, nil
	}
	rootPool := x509.NewCertPool()
	if ok := rootPool.AppendCertsFromPEM(certPEM); !ok {
		return nil, fmt.Errorf("invalid mongodb ssl certificate")
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    rootPool,
	}, nil
}

func mongoTLSCertificateFingerprint(ds datasource.DataSource) string {
	if !mongoTLSEnabled(ds) {
		return ""
	}
	certValue := strings.TrimSpace(optionString(ds.Options, "sslrootcert"))
	if certValue == "" {
		return ""
	}
	if certPEM, err := mongoRootCertificatePEM(ds.Options); err == nil && len(certPEM) > 0 {
		sum := sha256.Sum256(certPEM)
		return hex.EncodeToString(sum[:8])
	}
	sum := sha256.Sum256([]byte(certValue))
	return hex.EncodeToString(sum[:8])
}

func mongoRootCertificatePEM(options map[string]any) ([]byte, error) {
	certValue := strings.TrimSpace(optionString(options, "sslrootcert"))
	if certValue == "" {
		return nil, nil
	}
	if isPostgresPEMCertificate(certValue) {
		return []byte(certValue), nil
	}

	raw, err := os.ReadFile(certValue)
	if err != nil {
		return nil, fmt.Errorf("read mongodb ssl certificate file: %w", err)
	}
	normalized := strings.TrimSpace(string(raw))
	if normalized == "" {
		return nil, fmt.Errorf("invalid mongodb ssl certificate")
	}
	return []byte(normalized), nil
}

func ensureMongoTLSParam(uri string, enabled bool) string {
	if !enabled {
		return uri
	}
	lower := strings.ToLower(uri)
	if strings.Contains(lower, "tls=") || strings.Contains(lower, "ssl=") {
		return uri
	}
	sep := "?"
	if strings.Contains(uri, "?") {
		sep = "&"
	}
	return uri + sep + "tls=true"
}

func mongoDatabase(ds datasource.DataSource) (string, error) {
	if strings.TrimSpace(ds.Database) != "" {
		return ds.Database, nil
	}
	if ds.Options != nil {
		if uri, ok := ds.Options["uri"].(string); ok && strings.TrimSpace(uri) != "" {
			if db := mongoDatabaseFromURI(uri); db != "" {
				return db, nil
			}
		}
	}
	return "", errors.New("database is required")
}

func mongoStatementDatabase(ds datasource.DataSource, stmt mongoStatement) (string, error) {
	if strings.TrimSpace(stmt.Database) != "" {
		return strings.TrimSpace(stmt.Database), nil
	}
	return mongoDatabase(ds)
}

func mongoDatabaseFromURI(uri string) string {
	input := strings.TrimSpace(uri)
	if input == "" {
		return ""
	}
	schemeIdx := strings.Index(input, "://")
	if schemeIdx == -1 {
		return ""
	}
	rest := input[schemeIdx+3:]
	slash := strings.Index(rest, "/")
	if slash == -1 {
		return ""
	}
	path := rest[slash+1:]
	if path == "" {
		return ""
	}
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}
	if idx := strings.Index(path, "/"); idx != -1 {
		path = path[:idx]
	}
	return strings.TrimSpace(path)
}

func mongoKey(ds datasource.DataSource) string {
	certFingerprint := mongoTLSCertificateFingerprint(ds)
	// Use URI from options if available
	if ds.Options != nil {
		if uri, ok := ds.Options["uri"].(string); ok && strings.TrimSpace(uri) != "" {
			return fmt.Sprintf("%s|uri|%s|tls|%t|cert|%s", ds.ID, uri, mongoTLSEnabled(ds), certFingerprint)
		}
		// Use hosts for replica set mode
		if hostsRaw, ok := ds.Options["hosts"]; ok {
			var hostsStr string
			switch v := hostsRaw.(type) {
			case []string:
				hostsStr = strings.Join(v, ",")
			case []any:
				var parts []string
				for _, h := range v {
					if s, ok := h.(string); ok {
						parts = append(parts, s)
					}
				}
				hostsStr = strings.Join(parts, ",")
			}
			replicaSet, _ := ds.Options["replicaSet"].(string)
			return fmt.Sprintf(
				"%s|hosts|%s|%s|tls|%t|cert|%s|%s|%s|%s|%s",
				ds.ID,
				hostsStr,
				replicaSet,
				mongoTLSEnabled(ds),
				certFingerprint,
				ds.Username,
				ds.Password,
				ds.Database,
				ds.AuthSource,
			)
		}
	}
	// Single host mode
	return fmt.Sprintf(
		"%s|%s|%d|tls|%t|cert|%s|%s|%s|%s|%s",
		ds.ID,
		ds.Host,
		ds.Port,
		mongoTLSEnabled(ds),
		certFingerprint,
		ds.Username,
		ds.Password,
		ds.Database,
		ds.AuthSource,
	)
}
