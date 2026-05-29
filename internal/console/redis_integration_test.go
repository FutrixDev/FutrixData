package console

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"futrixdata/platform/internal/datasource"
)

func TestRedisAdapter_ExecuteQuotedAndBinarySafeCommandAgainstRedis(t *testing.T) {
	addr := os.Getenv("FUTRIXDATA_REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set FUTRIXDATA_REDIS_TEST_ADDR to run real Redis command parsing integration test")
	}
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("FUTRIXDATA_REDIS_TEST_ADDR must be host:port: %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("FUTRIXDATA_REDIS_TEST_ADDR has invalid port %q: %v", portText, err)
	}

	ds := datasource.DataSource{
		ID:       "redis-integration",
		Type:     datasource.TypeRedis,
		Host:     host,
		Port:     port,
		Password: os.Getenv("FUTRIXDATA_REDIS_TEST_PASSWORD"),
	}
	adapter := NewRedisAdapter()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := fmt.Sprintf("fd quoted key %d", time.Now().UnixNano())
	value := string([]byte{0x00, 'A', '\n', 'B'})
	_, err = adapter.Execute(ctx, ds, fmt.Sprintf(`SET "%s" "\x00A\nB"`, key), ExecuteOptions{})
	if err != nil {
		t.Fatalf("SET quoted/binary value: %v", err)
	}
	defer func() {
		_, _ = adapter.Execute(context.Background(), ds, fmt.Sprintf(`DEL "%s"`, key), ExecuteOptions{})
	}()

	result, err := adapter.Execute(ctx, ds, fmt.Sprintf(`GET "%s"`, key), ExecuteOptions{})
	if err != nil {
		t.Fatalf("GET quoted/binary value: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["result"] != value {
		t.Fatalf("GET result = %#v, want %q", result.Rows, value)
	}

	pathKey := fmt.Sprintf("fd single quoted path %d", time.Now().UnixNano())
	pathValue := "C:\\\\tmp"
	_, err = adapter.Execute(ctx, ds, fmt.Sprintf(`SET "%s" 'C:\\tmp'`, pathKey), ExecuteOptions{})
	if err != nil {
		t.Fatalf("SET single-quoted backslash value: %v", err)
	}
	defer func() {
		_, _ = adapter.Execute(context.Background(), ds, fmt.Sprintf(`DEL "%s"`, pathKey), ExecuteOptions{})
	}()

	pathResult, err := adapter.Execute(ctx, ds, fmt.Sprintf(`GET "%s"`, pathKey), ExecuteOptions{})
	if err != nil {
		t.Fatalf("GET single-quoted backslash value: %v", err)
	}
	if len(pathResult.Rows) != 1 || pathResult.Rows[0]["result"] != pathValue {
		t.Fatalf("GET path result = %#v, want %q", pathResult.Rows, pathValue)
	}
}
