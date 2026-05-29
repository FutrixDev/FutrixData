package paging

import (
	"encoding/base64"
	"encoding/json"
)

type Direction string

const (
	DirectionNext Direction = "next"
	DirectionPrev Direction = "prev"
)

type SortKey struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc"`
}

type Token struct {
	Version      int       `json:"v"`
	DatasourceID string    `json:"ds"`
	QueryHash    string    `json:"qh"`
	PageSize     int       `json:"ps"`
	Limit        int64     `json:"lim,omitempty"`
	Offset       int64     `json:"off,omitempty"`
	Sort         []SortKey `json:"sort"`
	StartCursor  []any     `json:"start"`
	EndCursor    []any     `json:"end"`
	Direction    Direction `json:"dir"`
}

func Encode(token Token) (string, error) {
	payload, err := json.Marshal(token)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func Decode(raw string) (Token, error) {
	var token Token
	if raw == "" {
		return token, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return token, err
	}
	err = json.Unmarshal(payload, &token)
	return token, err
}
