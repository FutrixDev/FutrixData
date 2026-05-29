package main

import (
	"errors"
	"strings"
	"time"

	"futrixdata/platform/internal/redisproto"
)

// RedisProtobufSchemaPayload is the wire-format input used by Wails bindings.
// Mirrors redisproto.SaveRequest but lives in main package to keep the JSON
// field names stable for the frontend.
type RedisProtobufSchemaPayload struct {
	ID           string `json:"id"`
	DatasourceID string `json:"datasourceId"`
	Name         string `json:"name"`
	Content      string `json:"content"`
}

// RedisProtobufSchemaView is the Wails-safe schema model exposed to the
// frontend. The store keeps time.Time values, but Wails' TS generator does not
// model time.Time cleanly for nested structs.
type RedisProtobufSchemaView struct {
	ID           string `json:"id"`
	DatasourceID string `json:"datasourceId"`
	Name         string `json:"name"`
	Content      string `json:"content"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

func redisProtobufSchemaView(schema redisproto.Schema) RedisProtobufSchemaView {
	return RedisProtobufSchemaView{
		ID:           schema.ID,
		DatasourceID: schema.DatasourceID,
		Name:         schema.Name,
		Content:      schema.Content,
		CreatedAt:    redisProtobufTimestamp(schema.CreatedAt),
		UpdatedAt:    redisProtobufTimestamp(schema.UpdatedAt),
	}
}

func redisProtobufSchemaViews(schemas []redisproto.Schema) []RedisProtobufSchemaView {
	out := make([]RedisProtobufSchemaView, 0, len(schemas))
	for _, schema := range schemas {
		out = append(out, redisProtobufSchemaView(schema))
	}
	return out
}

func redisProtobufTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func (a *App) ListRedisProtobufSchemas(datasourceID string) ([]RedisProtobufSchemaView, error) {
	if a.redisProtoStore == nil {
		return []RedisProtobufSchemaView{}, nil
	}
	id := strings.TrimSpace(datasourceID)
	if id == "" {
		// Empty selector lists everything — useful for the manage dialog.
		return redisProtobufSchemaViews(a.redisProtoStore.List()), nil
	}
	scoped := a.redisProtoStore.ListByDatasource(id)
	global := a.redisProtoStore.ListByDatasource("")
	out := make([]redisproto.Schema, 0, len(scoped)+len(global))
	out = append(out, scoped...)
	out = append(out, global...)
	return redisProtobufSchemaViews(out), nil
}

func (a *App) GetRedisProtobufSchema(id string) (RedisProtobufSchemaView, error) {
	if a.redisProtoStore == nil {
		return RedisProtobufSchemaView{}, errors.New("redis protobuf store unavailable")
	}
	schema, ok := a.redisProtoStore.Get(strings.TrimSpace(id))
	if !ok {
		return RedisProtobufSchemaView{}, redisproto.ErrNotFound
	}
	return redisProtobufSchemaView(schema), nil
}

func (a *App) SaveRedisProtobufSchema(payload RedisProtobufSchemaPayload) (RedisProtobufSchemaView, error) {
	if a.redisProtoStore == nil {
		return RedisProtobufSchemaView{}, errors.New("redis protobuf store unavailable")
	}
	schema, err := a.redisProtoStore.Save(redisproto.SaveRequest{
		ID:           strings.TrimSpace(payload.ID),
		DatasourceID: strings.TrimSpace(payload.DatasourceID),
		Name:         payload.Name,
		Content:      payload.Content,
	})
	if err != nil {
		return RedisProtobufSchemaView{}, err
	}
	return redisProtobufSchemaView(schema), nil
}

func (a *App) DeleteRedisProtobufSchema(id string) (bool, error) {
	if a.redisProtoStore == nil {
		return false, errors.New("redis protobuf store unavailable")
	}
	if err := a.redisProtoStore.Delete(strings.TrimSpace(id)); err != nil {
		return false, err
	}
	return true, nil
}
