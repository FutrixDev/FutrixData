package datasourceops

import (
	"fmt"
	"strings"

	"futrixdata/platform/internal/console"
	"futrixdata/platform/internal/datasource"
	"futrixdata/platform/internal/secrets"
)

type DataSourcePayload struct {
	Name       string                       `json:"name"`
	Type       datasource.DataSourceType    `json:"type"`
	Host       string                       `json:"host"`
	Port       int                          `json:"port"`
	Username   string                       `json:"username"`
	Password   string                       `json:"password"`
	Database   string                       `json:"database"`
	AuthSource string                       `json:"authSource"`
	Options    map[string]any               `json:"options"`
	SecretRefs map[string]secrets.SecretRef `json:"secretRefs,omitempty"`
}

func (p DataSourcePayload) ToDatasource(id string) datasource.DataSource {
	return datasource.DataSource{
		ID:         id,
		Name:       strings.TrimSpace(p.Name),
		Type:       p.Type,
		Host:       strings.TrimSpace(p.Host),
		Port:       p.Port,
		Username:   strings.TrimSpace(p.Username),
		Password:   p.Password,
		Database:   strings.TrimSpace(p.Database),
		AuthSource: strings.TrimSpace(p.AuthSource),
		Options:    p.Options,
		SecretRefs: datasource.PruneSecretRefs(p.SecretRefs),
	}
}

type RedisKeyPage struct {
	Keys   []string `json:"keys"`
	Cursor string   `json:"cursor"`
	Done   bool     `json:"done"`
}

type DatasourceMetrics struct {
	DatasourceID   string                    `json:"datasourceId"`
	DatasourceType datasource.DataSourceType `json:"datasourceType"`
	CollectedAt    int64                     `json:"collectedAt"`
	Node           string                    `json:"node,omitempty"`
	Nodes          []string                  `json:"nodes,omitempty"`

	CPUAvailable     bool    `json:"cpuAvailable"`
	CPUPercent       float64 `json:"cpuPercent,omitempty"`
	CPUUserSeconds   float64 `json:"cpuUserSeconds,omitempty"`
	CPUSystemSeconds float64 `json:"cpuSystemSeconds,omitempty"`

	MemoryAvailable  bool   `json:"memoryAvailable"`
	MemoryUsedBytes  int64  `json:"memoryUsedBytes,omitempty"`
	MemoryTotalBytes int64  `json:"memoryTotalBytes,omitempty"`
	MemoryUsedText   string `json:"memoryUsedText,omitempty"`
	MemoryTotalText  string `json:"memoryTotalText,omitempty"`

	Warnings []string       `json:"warnings,omitempty"`
	Raw      map[string]any `json:"raw,omitempty"`
}

type D1OAuthSession struct {
	Accounts  []D1OAuthAccount `json:"accounts"`
	AccountID string           `json:"accountId"`
	Token     string           `json:"token"`
}

type D1OAuthAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type D1CloudDatabase struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type DynamoDBSSOProfile struct {
	Name      string `json:"name"`
	Region    string `json:"region"`
	SSORegion string `json:"ssoRegion"`
	StartURL  string `json:"startUrl"`
	AccountID string `json:"accountId"`
	RoleName  string `json:"roleName"`
}

type DynamoDBSSOLoginResult struct {
	AccessToken string `json:"accessToken"`
	ExpiresAt   string `json:"expiresAt"`
}

type DynamoDBSSOAccount struct {
	AccountID    string `json:"accountId"`
	AccountName  string `json:"accountName"`
	EmailAddress string `json:"emailAddress"`
}

type DynamoDBSSORole struct {
	RoleName  string `json:"roleName"`
	AccountID string `json:"accountId"`
}

type DynamoDBSSORoleCredentials struct {
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken"`
	Expiration      int64  `json:"expiration"`
}

type DynamoDBSSOOAuthResult struct {
	Profile         string `json:"profile"`
	Region          string `json:"region"`
	AccountID       string `json:"accountId"`
	RoleName        string `json:"roleName"`
	AccessKeyID     string `json:"accessKeyId"`
	SecretAccessKey string `json:"secretAccessKey"`
	SessionToken    string `json:"sessionToken"`
	Expiration      int64  `json:"expiration"`
}

type schemaKnowledgeEntity struct {
	Name    string               `json:"name"`
	Columns []console.ColumnInfo `json:"columns,omitempty"`
	Indexes []console.IndexInfo  `json:"indexes,omitempty"`
	Details []console.DetailItem `json:"details,omitempty"`
}

func RedactValue(value any) any {
	return datasource.RedactValue(value)
}

func RedactDatasource(ds datasource.DataSource) datasource.DataSource {
	return datasource.RedactDatasource(ds)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func optionAnyString(options map[string]any, key string) string {
	if options == nil {
		return ""
	}
	raw, ok := options[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		rendered := strings.TrimSpace(fmt.Sprint(typed))
		if rendered == "<nil>" {
			return ""
		}
		return rendered
	}
}

func optionAnyBool(options map[string]any, key string) bool {
	if options == nil {
		return false
	}
	raw, ok := options[key]
	if !ok || raw == nil {
		return false
	}
	switch typed := raw.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	case int:
		return typed != 0
	case int8:
		return typed != 0
	case int16:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint:
		return typed != 0
	case uint8:
		return typed != 0
	case uint16:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	case float32:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}
