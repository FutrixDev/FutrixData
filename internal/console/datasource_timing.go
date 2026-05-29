package console

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"futrixdata/platform/internal/datasource"
)

type DatasourceTimingLogger interface {
	Printf(format string, v ...any)
}

type DatasourceTimingField struct {
	Key   string
	Value any
}

type DatasourceTimingMetadata struct {
	Entrypoint     string
	RequestID      string
	DatasourceID   string
	DatasourceType string
	Database       string
	Dialect        string
	StatementKind  string
	StatementHash  string
	StatementBytes int
	PageSize       int
	HasPagingToken bool
	UserApproved   bool
}

type DatasourceTimingTrace struct {
	logger DatasourceTimingLogger
	meta   DatasourceTimingMetadata
	start  time.Time
	mu     sync.Mutex
}

type datasourceTimingTraceKey struct{}

func DatasourceTimingKV(key string, value any) DatasourceTimingField {
	return DatasourceTimingField{Key: key, Value: value}
}

func DatasourceTimingErrorFields(err error) []DatasourceTimingField {
	if err == nil {
		return nil
	}
	return []DatasourceTimingField{
		DatasourceTimingKV("error_kind", datasourceTimingErrorKind(err)),
		DatasourceTimingKV("error_message", datasourceTimingSanitizeError(err.Error())),
	}
}

func DatasourceTimingStatusFields(err error, fields ...DatasourceTimingField) []DatasourceTimingField {
	result := []DatasourceTimingField{DatasourceTimingKV("status", timingStatus(err))}
	result = append(result, DatasourceTimingErrorFields(err)...)
	result = append(result, fields...)
	return result
}

func DatasourceTimingStatus(err error) string {
	return timingStatus(err)
}

func NewDatasourceTimingMetadata(entrypoint, requestID string, ds datasource.DataSource, statement string, opts ExecuteOptions, userApproved bool) DatasourceTimingMetadata {
	dialect := datasourceTimingDialectForDatasource(ds.Type)
	return DatasourceTimingMetadata{
		Entrypoint:     strings.TrimSpace(entrypoint),
		RequestID:      strings.TrimSpace(requestID),
		DatasourceID:   strings.TrimSpace(ds.ID),
		DatasourceType: strings.TrimSpace(string(ds.Type)),
		Database:       strings.TrimSpace(ds.Database),
		Dialect:        dialect,
		StatementKind:  datasourceTimingStatementKind(ds.Type, statement, dialect),
		StatementHash:  datasourceTimingStatementHash(statement),
		StatementBytes: len([]byte(statement)),
		PageSize:       opts.PageSize,
		HasPagingToken: strings.TrimSpace(opts.PagingToken) != "",
		UserApproved:   userApproved,
	}
}

func datasourceTimingStatementKind(dsType datasource.DataSourceType, statement, dialect string) string {
	statement = strings.TrimSpace(statement)
	if statement == "" {
		return ""
	}
	if dsType == datasource.TypeRedis || dsType == datasource.TypeRedisCluster {
		if strings.Contains(statement, "\n") {
			return "batch"
		}
		if fields := strings.Fields(statement); len(fields) > 0 {
			return strings.ToLower(strings.TrimSpace(fields[0]))
		}
	}
	return strings.ToLower(strings.TrimSpace(SQLStatementVerb(statement, dialect)))
}

func NewDatasourceTimingTrace(logger DatasourceTimingLogger, meta DatasourceTimingMetadata) *DatasourceTimingTrace {
	if logger == nil {
		return nil
	}
	if meta.Entrypoint == "" {
		meta.Entrypoint = "unknown"
	}
	if meta.StatementKind == "" {
		meta.StatementKind = "unknown"
	}
	if meta.Dialect == "" {
		meta.Dialect = "unknown"
	}
	return &DatasourceTimingTrace{logger: logger, meta: meta, start: time.Now()}
}

func WithDatasourceTimingTrace(ctx context.Context, trace *DatasourceTimingTrace) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if trace == nil {
		return ctx
	}
	return context.WithValue(ctx, datasourceTimingTraceKey{}, trace)
}

func DatasourceTimingTraceFromContext(ctx context.Context) *DatasourceTimingTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(datasourceTimingTraceKey{}).(*DatasourceTimingTrace)
	return trace
}

func DatasourceTimingStage(ctx context.Context, stage string) func(...DatasourceTimingField) {
	trace := DatasourceTimingTraceFromContext(ctx)
	if trace == nil {
		return func(...DatasourceTimingField) {}
	}
	start := time.Now()
	return func(fields ...DatasourceTimingField) {
		trace.LogStage(stage, time.Since(start), fields...)
	}
}

func DatasourceTimingEvent(ctx context.Context, event string, fields ...DatasourceTimingField) {
	trace := DatasourceTimingTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.log(event, "", 0, fields...)
}

func (t *DatasourceTimingTrace) LogStage(stage string, duration time.Duration, fields ...DatasourceTimingField) {
	if t == nil {
		return
	}
	t.log("stage", strings.TrimSpace(stage), duration, fields...)
}

func (t *DatasourceTimingTrace) Finish(status string, fields ...DatasourceTimingField) {
	if t == nil {
		return
	}
	if strings.TrimSpace(status) == "" {
		status = "ok"
	}
	t.log("finish", "total", time.Since(t.start), append([]DatasourceTimingField{DatasourceTimingKV("status", status)}, fields...)...)
}

func (t *DatasourceTimingTrace) log(event, stage string, duration time.Duration, fields ...DatasourceTimingField) {
	if t == nil || t.logger == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	parts := []string{
		"source=datasource_timing",
		"event=" + timingValue(event),
		"ts=" + timingValue(now.UTC().Format(time.RFC3339Nano)),
		"request_id=" + timingValue(t.meta.RequestID),
		"entrypoint=" + timingValue(t.meta.Entrypoint),
	}
	if stage != "" {
		parts = append(parts, "stage="+timingValue(stage))
	}
	parts = append(parts,
		"duration_ms="+timingDuration(duration),
		"total_ms="+timingDuration(now.Sub(t.start)),
		"datasource_id="+timingValue(t.meta.DatasourceID),
		"datasource_type="+timingValue(t.meta.DatasourceType),
		"database="+timingValue(t.meta.Database),
		"dialect="+timingValue(t.meta.Dialect),
		"statement_kind="+timingValue(t.meta.StatementKind),
		"statement_hash="+timingValue(t.meta.StatementHash),
		fmt.Sprintf("statement_bytes=%d", t.meta.StatementBytes),
		fmt.Sprintf("page_size=%d", t.meta.PageSize),
		fmt.Sprintf("has_paging_token=%t", t.meta.HasPagingToken),
		fmt.Sprintf("user_approved=%t", t.meta.UserApproved),
	)
	for _, field := range fields {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			continue
		}
		parts = append(parts, key+"="+timingValue(field.Value))
	}
	t.logger.Printf(strings.Join(parts, " "))
}

func datasourceTimingDialectForDatasource(dsType datasource.DataSourceType) string {
	switch dsType {
	case datasource.TypeMySQL:
		return "mysql"
	case datasource.TypePostgreSQL:
		return "postgres"
	case datasource.TypeD1:
		return "d1"
	default:
		return strings.ToLower(strings.TrimSpace(string(dsType)))
	}
}

func datasourceTimingStatementHash(statement string) string {
	if strings.TrimSpace(statement) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(statement))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func timingDuration(duration time.Duration) string {
	if duration <= 0 {
		return "0.000"
	}
	return fmt.Sprintf("%.3f", float64(duration.Microseconds())/1000)
}

func timingValue(value any) string {
	switch v := value.(type) {
	case nil:
		return `""`
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%.3f", v)
	case time.Duration:
		return timingDuration(v)
	case string:
		return timingQuote(v)
	default:
		return timingQuote(fmt.Sprint(v))
	}
}

func timingQuote(value string) string {
	raw, _ := json.Marshal(strings.TrimSpace(value))
	return string(raw)
}

func timingStatus(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func datasourceTimingErrorKind(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "timeout"
		}
		return "network"
	}
	if _, ok := RiskInfoFromError(err); ok {
		return "risk_guard"
	}
	return "error"
}

var (
	datasourceTimingCredentialInURLPattern = regexp.MustCompile(`(?i)(://[^:/\s]+:)[^@\s]+(@)`)
	datasourceTimingAuthorizationPattern   = regexp.MustCompile(`(?i)\bAuthorization\s*[:=]\s*(Bearer\s+)?[^\s,;]+`)
	datasourceTimingSecretKeyPattern       = regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api[_-]?key|access[_-]?key)\s*[:=]\s*('[^']*'|"[^"]*"|[^\s,;]+)`)
	datasourceTimingBearerPattern          = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/\-=]+`)
)

func datasourceTimingSanitizeError(message string) string {
	message = strings.TrimSpace(strings.Join(strings.Fields(message), " "))
	if message == "" {
		return ""
	}
	message = datasourceTimingCredentialInURLPattern.ReplaceAllString(message, `${1}[redacted]${2}`)
	message = datasourceTimingAuthorizationPattern.ReplaceAllString(message, `Authorization=[redacted]`)
	message = datasourceTimingBearerPattern.ReplaceAllString(message, `Bearer [redacted]`)
	message = datasourceTimingSecretKeyPattern.ReplaceAllString(message, `${1}=[redacted]`)
	const maxRunes = 240
	runes := []rune(message)
	if len(runes) > maxRunes {
		message = string(runes[:maxRunes]) + "...[truncated]"
	}
	return message
}
