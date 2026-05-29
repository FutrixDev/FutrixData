package console

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"futrixdata/platform/internal/datasource"
)

func TestParseRedisCommand_PreservesQuotedAndBinarySafeArgs(t *testing.T) {
	got, err := parseRedisCommand(`SET "user profile" "\x00A\nB"`)
	if err != nil {
		t.Fatalf("parseRedisCommand returned error: %v", err)
	}
	want := []any{"SET", "user profile", string([]byte{0x00, 'A', '\n', 'B'})}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseRedisCommand = %#v, want %#v", got, want)
	}
}

func TestRedisClient_RedisForceStandaloneOption(t *testing.T) {
	ds := datasource.DataSource{
		Options: map[string]any{
			"__forceStandalone": true,
		},
	}
	if !redisForceStandalone(ds) {
		t.Fatalf("expected redisForceStandalone to be true")
	}
}

func TestRedisClient_RedisForceStandaloneParsesStringValues(t *testing.T) {
	ds := datasource.DataSource{
		Options: map[string]any{
			"__forceStandalone": "true",
		},
	}
	if !redisForceStandalone(ds) {
		t.Fatalf("expected redisForceStandalone to parse string true")
	}

	ds.Options["__forceStandalone"] = "0"
	if redisForceStandalone(ds) {
		t.Fatalf("expected redisForceStandalone to parse string 0 as false")
	}
}

func TestRedisCommandStatementQuotesArgsForAuditAndRiskText(t *testing.T) {
	got, err := RedisCommandStatement([]string{"SET", "user:1", "name with spaces"})
	if err != nil {
		t.Fatalf("RedisCommandStatement returned error: %v", err)
	}
	if got != `SET user:1 "name with spaces"` {
		t.Fatalf("statement = %q", got)
	}
}

func TestRedisArgsToAnyPreservesValueWhitespace(t *testing.T) {
	got, err := redisArgsToAny([]string{"SET", "user:1", "name with spaces"})
	if err != nil {
		t.Fatalf("redisArgsToAny returned error: %v", err)
	}
	parts := make([]string, len(got))
	for i, item := range got {
		parts[i] = item.(string)
	}
	if strings.Join(parts, "\x00") != strings.Join([]string{"SET", "user:1", "name with spaces"}, "\x00") {
		t.Fatalf("args = %#v", parts)
	}
}

func TestRedisClient_NewRedisClient_ForceStandaloneStillRetriesWithoutPasswordOnAuthError(t *testing.T) {
	originalNewClient := redisNewClient
	originalDetectMode := redisDetectMode
	defer func() {
		redisNewClient = originalNewClient
		redisDetectMode = originalDetectMode
	}()

	passwordByClient := make(map[*redis.Client]string)
	redisNewClient = func(opts *redis.Options) *redis.Client {
		client := redis.NewClient(opts)
		passwordByClient[client] = opts.Password
		return client
	}

	detectCalls := 0
	redisDetectMode = func(_ context.Context, client *redis.Client, _ datasource.DataSource) (redisConnInfo, error) {
		detectCalls++
		if passwordByClient[client] != "" {
			return redisConnInfo{}, errors.New("ERR Client sent AUTH, but no password is set")
		}
		return redisConnInfo{Mode: redisModeStandalone}, nil
	}

	adapter := &RedisAdapter{}
	ds := datasource.DataSource{
		Type:     datasource.TypeRedis,
		Host:     "127.0.0.1",
		Port:     6379,
		Username: "default",
		Password: "configured-but-server-has-no-auth",
		Options: map[string]any{
			"__forceStandalone": true,
		},
	}

	client, info, err := adapter.newRedisClient(context.Background(), ds)
	if err != nil {
		t.Fatalf("newRedisClient returned error: %v", err)
	}
	defer func() { _ = client.Close() }()

	if detectCalls != 2 {
		t.Fatalf("expected detect mode to run twice for auth retry, got %d", detectCalls)
	}
	typedClient, ok := client.(*redis.Client)
	if !ok {
		t.Fatalf("expected standalone redis client, got %T", client)
	}
	if gotPassword := passwordByClient[typedClient]; gotPassword != "" {
		t.Fatalf("expected retried client password to be cleared, got %q", gotPassword)
	}
	if info.Mode != redisModeStandalone {
		t.Fatalf("expected standalone mode, got %q", info.Mode)
	}
}
