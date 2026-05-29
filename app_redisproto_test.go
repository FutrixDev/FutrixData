package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"futrixdata/platform/internal/redisproto"
)

func TestRedisProtobufSchemaWailsViewUsesStringTimestamps(t *testing.T) {
	app := &App{
		redisProtoStore: redisproto.NewStore(filepath.Join(t.TempDir(), "redis-protobuf.json")),
	}

	schema, err := app.SaveRedisProtobufSchema(RedisProtobufSchemaPayload{
		DatasourceID: "redis_local",
		Name:         "user.proto",
		Content:      `syntax = "proto3"; message User { string name = 1; }`,
	})
	if err != nil {
		t.Fatalf("SaveRedisProtobufSchema returned error: %v", err)
	}

	if got := reflect.TypeOf(schema.CreatedAt).Kind(); got != reflect.String {
		t.Fatalf("CreatedAt should be a Wails-safe string timestamp, got %s", got)
	}
	if got := reflect.TypeOf(schema.UpdatedAt).Kind(); got != reflect.String {
		t.Fatalf("UpdatedAt should be a Wails-safe string timestamp, got %s", got)
	}
}
