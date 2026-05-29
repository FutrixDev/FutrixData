package console

import (
	"context"
	"encoding/base64"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

// looksBinary returns true when the raw string contains bytes that the JS side
// cannot faithfully render as text (NULs, control bytes other than \t\n\r, or
// any invalid UTF-8 sequence). Bitmap values produced by SETBIT and any
// binary-encoded protobuf/avro payload trigger this branch — the frontend
// shows them as a hex dump instead of garbled mojibake.
func looksBinary(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == '\t' || b == '\n' || b == '\r' {
			continue
		}
		if b < 0x20 || b == 0x7f {
			return true
		}
	}
	return false
}

func redisKeySize(ctx context.Context, client redis.UniversalClient, keyType, key string) (int64, bool) {
	switch keyType {
	case "string":
		value, err := client.StrLen(ctx, key).Result()
		if err != nil {
			return 0, false
		}
		return value, true
	case "list":
		value, err := client.LLen(ctx, key).Result()
		if err != nil {
			return 0, false
		}
		return value, true
	case "set":
		value, err := client.SCard(ctx, key).Result()
		if err != nil {
			return 0, false
		}
		return value, true
	case "zset":
		value, err := client.ZCard(ctx, key).Result()
		if err != nil {
			return 0, false
		}
		return value, true
	case "hash":
		value, err := client.HLen(ctx, key).Result()
		if err != nil {
			return 0, false
		}
		return value, true
	case "stream":
		value, err := client.XLen(ctx, key).Result()
		if err != nil {
			return 0, false
		}
		return value, true
	default:
		return 0, false
	}
}

func redisPreview(ctx context.Context, client redis.UniversalClient, keyType, key string, size int64) (map[string]any, error) {
	const previewLimit = int64(20)
	switch keyType {
	case "string":
		value, err := client.Get(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		truncated, sample := truncatePreviewValue(value, 512)
		out := map[string]any{
			"kind":      keyType,
			"limit":     previewLimit,
			"value":     sample,
			"truncated": truncated || (size > previewLimit),
		}
		if looksBinary(value) {
			// truncatePreviewValue may have appended a "...(truncated)" marker;
			// for the b64 payload we want the raw byte prefix only. 64 KiB is
			// enough to cover virtually every bitmap/protobuf payload users
			// actually want to inspect raw, while keeping the WebKit-bridge
			// payload bounded.
			const binaryB64Cap = 65536
			raw := value
			valueB64Truncated := false
			if len(raw) > binaryB64Cap {
				raw = raw[:binaryB64Cap]
				valueB64Truncated = true
			}
			out["binary"] = true
			out["valueB64"] = base64.StdEncoding.EncodeToString([]byte(raw))
			out["valueB64Truncated"] = valueB64Truncated
		}
		return out, nil
	case "hash":
		items, truncated, err := redisScanHash(ctx, client, key, previewLimit)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"kind":      keyType,
			"limit":     previewLimit,
			"items":     items,
			"truncated": truncated || (size > previewLimit),
		}, nil
	case "list":
		values, err := client.LRange(ctx, key, 0, previewLimit-1).Result()
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(values))
		for i, value := range values {
			items = append(items, map[string]any{"index": i, "value": value})
		}
		return map[string]any{
			"kind":      keyType,
			"limit":     previewLimit,
			"items":     items,
			"truncated": size > previewLimit,
		}, nil
	case "set":
		items, truncated, err := redisScanSet(ctx, client, key, previewLimit)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"kind":      keyType,
			"limit":     previewLimit,
			"items":     items,
			"truncated": truncated || (size > previewLimit),
		}, nil
	case "zset":
		values, err := client.ZRangeWithScores(ctx, key, 0, previewLimit-1).Result()
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(values))
		for _, value := range values {
			items = append(items, map[string]any{"value": value.Member, "score": value.Score})
		}
		return map[string]any{
			"kind":      keyType,
			"limit":     previewLimit,
			"items":     items,
			"truncated": size > previewLimit,
		}, nil
	case "stream":
		values, err := client.XRangeN(ctx, key, "-", "+", previewLimit).Result()
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(values))
		for _, value := range values {
			items = append(items, map[string]any{"id": value.ID, "fields": value.Values})
		}
		return map[string]any{
			"kind":      keyType,
			"limit":     previewLimit,
			"items":     items,
			"truncated": size > previewLimit,
		}, nil
	default:
		return nil, nil
	}
}

func truncatePreviewValue(value string, limit int) (bool, string) {
	if limit <= 0 || len(value) <= limit {
		return false, value
	}
	return true, value[:limit] + "...(truncated)"
}

func redisScanHash(ctx context.Context, client redis.UniversalClient, key string, limit int64) ([]map[string]any, bool, error) {
	values, cursor, err := client.HScan(ctx, key, 0, "*", limit).Result()
	if err != nil {
		return nil, false, err
	}
	items := make([]map[string]any, 0, len(values)/2)
	for i := 0; i+1 < len(values); i += 2 {
		items = append(items, map[string]any{"field": values[i], "value": values[i+1]})
	}
	return items, cursor != 0, nil
}

func redisScanSet(ctx context.Context, client redis.UniversalClient, key string, limit int64) ([]map[string]any, bool, error) {
	values, cursor, err := client.SScan(ctx, key, 0, "*", limit).Result()
	if err != nil {
		return nil, false, err
	}
	items := make([]map[string]any, 0, len(values))
	for _, value := range values {
		items = append(items, map[string]any{"value": value})
	}
	return items, cursor != 0, nil
}

func formatTTL(ttl time.Duration) string {
	switch {
	case ttl < 0:
		if ttl == -2*time.Second {
			return "missing"
		}
		return "no-expire"
	default:
		return ttl.String()
	}
}
