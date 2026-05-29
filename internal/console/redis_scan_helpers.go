package console

import "strings"

func redisScanCount(pattern string) int64 {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return 2000
	}
	return 10000
}

func redisPrefixPattern(pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return false
	}
	if strings.ContainsAny(pattern, "?[") {
		return false
	}
	if strings.Count(pattern, "*") != 1 {
		return false
	}
	if strings.HasPrefix(pattern, "*") {
		return false
	}
	return strings.HasSuffix(pattern, "*")
}
