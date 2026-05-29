package redisproto

import "time"

// Schema represents a persisted .proto file that the redis console can use
// to decode protobuf-encoded values. The Content field stores the original
// .proto text; parsing happens on the frontend via protobufjs.
type Schema struct {
	ID           string    `json:"id"`
	DatasourceID string    `json:"datasourceId"`
	Name         string    `json:"name"`
	Content      string    `json:"content"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// SaveRequest is the payload used by Create and Update. When ID is empty
// a new schema is created; otherwise an existing one is updated.
type SaveRequest struct {
	ID           string `json:"id"`
	DatasourceID string `json:"datasourceId"`
	Name         string `json:"name"`
	Content      string `json:"content"`
}
