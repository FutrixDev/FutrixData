package riskengine

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	operationClassRead   = "read"
	operationClassWrite  = "write"
	operationClassAdmin  = "admin"
	operationClassScan   = "scan"
	operationClassScript = "script"
)

var redisLuaCallCommandPattern = regexp.MustCompile(`(?is)\bredis\s*\.\s*(?:call|pcall)\s*\(\s*['"]([A-Za-z][A-Za-z0-9_.-]*)['"]`)

func redisOperationIntent(command string, args []string) OperationIntent {
	command = strings.ToLower(strings.TrimSpace(command))
	intent := OperationIntent{
		Command:           command,
		CommandCandidates: uniqueLowerNonEmpty(command),
		Args:              append([]string(nil), args...),
		Classes:           redisCommandClasses(command),
	}

	keys := redisKeyCandidates(command, args)
	if redisCommandHasClass(command, operationClassScript) {
		intent.Classes = appendUniqueLower(intent.Classes, operationClassScript)
		intent.RedisScript = &RedisScriptIntent{Present: true}
		keys = redisScriptKeyCandidates(command, args)
		allowsWrites := redisScriptCommandAllowsWrites(command)
		for _, inner := range redisLuaInnerCommands(redisScriptSource(command, args)) {
			innerClasses := redisCommandClasses(inner)
			executableInnerCommand := allowsWrites || !redisCommandHasClass(inner, operationClassWrite)
			if executableInnerCommand {
				intent.CommandCandidates = appendUniqueLower(intent.CommandCandidates, inner)
				intent.RedisScript.InnerCommands = appendUniqueLower(intent.RedisScript.InnerCommands, inner)
			}
			if !allowsWrites {
				innerClasses = removeOperationClass(innerClasses, operationClassWrite)
			}
			intent.Classes = appendUniqueLower(intent.Classes, innerClasses...)
		}
	}
	intent.KeyCandidates = uniqueNonEmpty(keys...)
	return intent
}

func redisScriptCommandAllowsWrites(command string) bool {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "eval_ro", "evalsha_ro", "fcall_ro":
		return false
	default:
		return true
	}
}

func removeOperationClass(classes []string, remove string) []string {
	out := classes[:0]
	for _, class := range classes {
		if !strings.EqualFold(class, remove) {
			out = append(out, class)
		}
	}
	return out
}

func redisScriptSource(command string, args []string) string {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "eval", "eval_ro":
		if len(args) > 0 {
			return args[0]
		}
	}
	return ""
}

func redisScriptKeyCandidates(command string, args []string) []string {
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "eval", "evalsha", "eval_ro", "evalsha_ro", "fcall", "fcall_ro":
	default:
		return nil
	}
	if len(args) < 2 {
		return nil
	}
	count, err := strconv.Atoi(strings.TrimSpace(args[1]))
	if err != nil || count <= 0 {
		return nil
	}
	start := 2
	end := start + count
	if end > len(args) {
		end = len(args)
	}
	return append([]string(nil), args[start:end]...)
}

func redisLuaInnerCommands(script string) []string {
	var commands []string
	for _, match := range redisLuaCallCommandPattern.FindAllStringSubmatch(script, -1) {
		if len(match) > 1 {
			commands = appendUniqueLower(commands, match[1])
		}
	}
	return commands
}

func redisKeyCandidates(command string, args []string) []string {
	if len(args) == 0 {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(command)) {
	case "del", "unlink", "exists", "mget", "touch":
		return append([]string(nil), args...)
	case "mset", "msetnx":
		keys := make([]string, 0, (len(args)+1)/2)
		for i := 0; i < len(args); i += 2 {
			keys = append(keys, args[i])
		}
		return keys
	case "rename", "renamenx", "copy", "move", "smove", "rpoplpush", "lmove":
		if len(args) >= 2 {
			return []string{args[0], args[1]}
		}
	}
	return []string{args[0]}
}

func redisCommandClasses(command string) []string {
	var classes []string
	for _, class := range []string{operationClassWrite, operationClassAdmin, operationClassScan, operationClassScript, operationClassRead} {
		if redisCommandHasClass(command, class) {
			classes = append(classes, class)
		}
	}
	return classes
}

func redisCommandHasClass(command, class string) bool {
	command = strings.ToLower(strings.TrimSpace(command))
	switch class {
	case operationClassRead:
		return RedisCommandIsLowRisk(command)
	case operationClassScript:
		switch command {
		case "eval", "evalsha", "eval_ro", "evalsha_ro", "fcall", "fcall_ro":
			return true
		}
	case operationClassAdmin:
		switch command {
		case "flushall", "flushdb", "shutdown", "debug", "slaveof", "replicaof", "failover",
			"swapdb", "bgrewriteaof", "config", "cluster", "acl", "function", "module",
			"client", "migrate", "memory", "slowlog":
			return true
		}
	case operationClassScan:
		switch command {
		case "keys", "scan", "sscan", "hscan", "zscan", "hgetall", "smembers",
			"zrange", "zrevrange", "zrangebyscore", "zrevrangebyscore", "zrangebylex",
			"lrange", "xrange", "xrevrange", "xread", "sort":
			return true
		}
	case operationClassWrite:
		switch command {
		case "flushall", "flushdb",
			"del", "unlink",
			"set", "mset", "msetnx", "setnx", "setex", "psetex", "append", "setrange", "getset", "getdel",
			"incr", "decr", "incrby", "decrby", "incrbyfloat",
			"hset", "hmset", "hsetnx", "hdel", "hincrby", "hincrbyfloat",
			"lpush", "rpush", "lpushx", "rpushx", "lpop", "rpop", "lset", "linsert", "lrem", "ltrim", "rpoplpush", "lmove", "lmpop",
			"sadd", "srem", "spop", "smove", "sdiffstore", "sinterstore", "sunionstore",
			"zadd", "zrem", "zincrby", "zpopmin", "zpopmax", "zrangestore", "zdiffstore", "zinterstore", "zunionstore", "zmpop",
			"xadd", "xdel", "xtrim", "xack",
			"expire", "pexpire", "expireat", "pexpireat", "persist",
			"rename", "renamenx",
			"copy", "move",
			"pfadd", "pfmerge",
			"geoadd", "georadius", "georadiusbymember",
			"bitop", "bitfield", "setbit":
			return true
		}
	}
	return false
}

func operationHasAnyClass(intent OperationIntent, classes ...string) bool {
	for _, class := range classes {
		if operationHasClass(intent, class) {
			return true
		}
	}
	return false
}

func operationHasClass(intent OperationIntent, class string) bool {
	needle := strings.ToLower(strings.TrimSpace(class))
	for _, item := range intent.Classes {
		if strings.ToLower(strings.TrimSpace(item)) == needle {
			return true
		}
	}
	return false
}

func uniqueLowerNonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	return appendUniqueLower(out, values...)
}

func appendUniqueLower(out []string, values ...string) []string {
	seen := make(map[string]struct{}, len(out)+len(values))
	for _, item := range out {
		seen[item] = struct{}{}
	}
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		out = append(out, normalized)
		seen[normalized] = struct{}{}
	}
	return out
}

func uniqueNonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}
		out = append(out, trimmed)
		seen[key] = struct{}{}
	}
	return out
}
