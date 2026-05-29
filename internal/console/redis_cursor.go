package console

import (
	"encoding/json"
	"strings"
)

type RedisScanCursor struct {
	Cursor  uint64            `json:"cursor,omitempty"`
	Cursors map[string]uint64 `json:"cursors,omitempty"`
}

func EncodeRedisCursor(cursor RedisScanCursor) (string, error) {
	if cursor.Cursor == 0 && len(cursor.Cursors) == 0 {
		return "", nil
	}
	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func DecodeRedisCursor(value string) (RedisScanCursor, error) {
	if strings.TrimSpace(value) == "" {
		return RedisScanCursor{}, nil
	}
	var cursor RedisScanCursor
	if err := json.Unmarshal([]byte(value), &cursor); err != nil {
		return RedisScanCursor{}, err
	}
	if cursor.Cursors == nil {
		cursor.Cursors = map[string]uint64{}
	}
	return cursor, nil
}

func redisCursorDone(cursor RedisScanCursor) bool {
	if len(cursor.Cursors) == 0 {
		return cursor.Cursor == 0
	}
	for _, value := range cursor.Cursors {
		if value != 0 {
			return false
		}
	}
	return true
}
