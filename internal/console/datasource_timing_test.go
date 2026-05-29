package console

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"futrixdata/platform/internal/datasource"
)

func TestDatasourceTimingTraceLogsMetadataWithoutRawStatement(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	statement := "SELECT * FROM users WHERE email = 'secret@example.com'"
	ds := datasource.DataSource{ID: "mysql-prod", Type: datasource.TypeMySQL, Database: "crm"}
	meta := NewDatasourceTimingMetadata("test.execute", "req123", ds, statement, ExecuteOptions{PageSize: 50}, true)
	trace := NewDatasourceTimingTrace(logger, meta)
	ctx := WithDatasourceTimingTrace(context.Background(), trace)

	done := DatasourceTimingStage(ctx, "sql.query_context")
	time.Sleep(time.Millisecond)
	done(DatasourceTimingKV("status", "ok"), DatasourceTimingKV("rows", 1))
	trace.Finish("ok")

	content := buf.String()
	for _, want := range []string{
		"source=datasource_timing",
		"event=\"stage\"",
		"stage=\"sql.query_context\"",
		"ts=",
		"request_id=\"req123\"",
		"entrypoint=\"test.execute\"",
		"datasource_type=\"mysql\"",
		"database=\"crm\"",
		"statement_kind=\"select\"",
		"statement_hash=\"sha256:",
		"page_size=50",
		"user_approved=true",
		"rows=1",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("timing log missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "secret@example.com") || strings.Contains(content, "SELECT *") {
		t.Fatalf("timing log leaked raw statement:\n%s", content)
	}
}

func TestDatasourceTimingMetadataClassifiesRedisBatch(t *testing.T) {
	ds := datasource.DataSource{ID: "redis-dev", Type: datasource.TypeRedis}
	meta := NewDatasourceTimingMetadata("test.redis_batch", "req123", ds, "0: GET cache-key\n1: TTL cache-key", ExecuteOptions{}, false)
	if meta.StatementKind != "batch" {
		t.Fatalf("StatementKind = %q, want batch", meta.StatementKind)
	}
}

func TestDatasourceTimingErrorFieldsSanitizeAndClassify(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	ds := datasource.DataSource{ID: "d1-prod", Type: datasource.TypeD1, Database: "prod"}
	trace := NewDatasourceTimingTrace(logger, NewDatasourceTimingMetadata("test.connection", "req456", ds, "", ExecuteOptions{}, false))
	ctx := WithDatasourceTimingTrace(context.Background(), trace)

	err := errors.New(`connect failed password="secret" token=abc123 https://user:pass@example.test/db Authorization: Bearer abc.def`)
	done := DatasourceTimingStage(ctx, "manager.test_connection.adapter_call")
	done(DatasourceTimingStatusFields(err)...)
	trace.Finish(DatasourceTimingStatus(err), DatasourceTimingErrorFields(err)...)

	content := buf.String()
	for _, want := range []string{
		`status="error"`,
		`error_kind="error"`,
		`error_message=`,
		`password=[redacted]`,
		`token=[redacted]`,
		`https://user:[redacted]@example.test/db`,
		`Authorization=[redacted]`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("timing error log missing %q in:\n%s", want, content)
		}
	}
	for _, leaked := range []string{"secret", "abc123", "user:pass", "abc.def"} {
		if strings.Contains(content, leaked) {
			t.Fatalf("timing error log leaked %q in:\n%s", leaked, content)
		}
	}
}

func TestDatasourceTimingErrorKindTimeout(t *testing.T) {
	fields := DatasourceTimingErrorFields(context.DeadlineExceeded)
	if len(fields) == 0 || fields[0].Key != "error_kind" || fields[0].Value != "timeout" {
		t.Fatalf("timeout error fields = %#v, want error_kind timeout", fields)
	}
}
