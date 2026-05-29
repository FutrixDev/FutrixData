package console

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"futrixdata/platform/internal/datasource"
)

// RedisKeyMetaItem is the per-key metadata returned by GetKeyMeta. It covers
// the three pieces of information the Redis console tree needs to render a key
// row without follow-up RPCs: its type, TTL (in ms), and size (elements for
// containers, length for strings).
type RedisKeyMetaItem struct {
	// Type is the lowercase Redis type ("string", "hash", "list", "set",
	// "zset", "stream") or "none" if the key does not exist.
	Type string `json:"type"`

	// TTLMS is remaining time-to-live in milliseconds. -2 means the key does
	// not exist (or expired between our two pipeline passes), -1 means the
	// key has no expiry set.
	TTLMS int64 `json:"ttlMs"`

	// Size is the per-type size of the key:
	//   string: STRLEN (bytes)
	//   hash:   HLEN   (field count)
	//   list:   LLEN   (element count)
	//   set:    SCARD  (element count)
	//   zset:   ZCARD  (element count)
	//   stream: XLEN   (entry count)
	// 0 when the type is unknown / missing.
	Size int64 `json:"size"`
}

// GetKeyMeta returns Type/TTL/Size for each requested key using two Redis
// pipeline round-trips regardless of the number of keys. Missing keys come
// back with Type="none", TTLMS=-2, Size=0 so the caller can still render them.
func (r *RedisAdapter) GetKeyMeta(ctx context.Context, ds datasource.DataSource, keys []string) (map[string]RedisKeyMetaItem, error) {
	out := make(map[string]RedisKeyMetaItem, len(keys))
	if len(keys) == 0 {
		return out, nil
	}

	client, _, err := r.clientFor(ctx, ds)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	uniqueKeys := dedupeKeys(keys)

	typeCmds := make(map[string]*redis.StatusCmd, len(uniqueKeys))
	ttlCmds := make(map[string]*redis.DurationCmd, len(uniqueKeys))
	if _, err := client.Pipelined(ctx, func(p redis.Pipeliner) error {
		for _, key := range uniqueKeys {
			typeCmds[key] = p.Type(ctx, key)
			ttlCmds[key] = p.PTTL(ctx, key)
		}
		return nil
	}); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	types := make(map[string]string, len(uniqueKeys))
	ttls := make(map[string]int64, len(uniqueKeys))
	for _, key := range uniqueKeys {
		types[key] = resolveTypeCmd(typeCmds[key])
		ttls[key] = resolvePTTLCmd(ttlCmds[key])
	}

	sizeCmds := make(map[string]*redis.IntCmd, len(uniqueKeys))
	if _, err := client.Pipelined(ctx, func(p redis.Pipeliner) error {
		for _, key := range uniqueKeys {
			switch types[key] {
			case "string":
				sizeCmds[key] = p.StrLen(ctx, key)
			case "hash":
				sizeCmds[key] = p.HLen(ctx, key)
			case "list":
				sizeCmds[key] = p.LLen(ctx, key)
			case "set":
				sizeCmds[key] = p.SCard(ctx, key)
			case "zset":
				sizeCmds[key] = p.ZCard(ctx, key)
			case "stream":
				sizeCmds[key] = p.XLen(ctx, key)
			}
		}
		return nil
	}); err != nil && !errors.Is(err, redis.Nil) {
		// A partial failure in pass 2 should not blank out type+ttl info we
		// already have. Sizes that did not resolve fall through to 0.
	}

	for _, key := range uniqueKeys {
		size := int64(0)
		if cmd := sizeCmds[key]; cmd != nil && cmd.Err() == nil {
			size = cmd.Val()
		}
		out[key] = RedisKeyMetaItem{Type: types[key], TTLMS: ttls[key], Size: size}
	}

	return out, nil
}

func resolveTypeCmd(cmd *redis.StatusCmd) string {
	if cmd == nil {
		return "none"
	}
	if err := cmd.Err(); err != nil && !errors.Is(err, redis.Nil) {
		return "none"
	}
	val := cmd.Val()
	if val == "" {
		return "none"
	}
	return val
}

func resolvePTTLCmd(cmd *redis.DurationCmd) int64 {
	if cmd == nil {
		return -2
	}
	if err := cmd.Err(); err != nil && !errors.Is(err, redis.Nil) {
		return -2
	}
	d := cmd.Val()
	switch {
	case d == -2*time.Millisecond:
		return -2
	case d == -1*time.Millisecond:
		return -1
	default:
		return int64(d / time.Millisecond)
	}
}

func dedupeKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}
