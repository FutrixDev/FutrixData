package console

import (
	"reflect"
	"testing"

	"futrixdata/platform/internal/datasource"
)

func TestRedisTopology_NormalizeRedisNodesStripsBusPort(t *testing.T) {
	nodes := []string{
		"127.0.0.1:7000@17000",
		"127.0.0.1:7000",
		" 127.0.0.1:7001@17001 ",
	}
	expected := []string{"127.0.0.1:7000", "127.0.0.1:7001"}
	got := normalizeRedisNodes(nodes)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected %#v, got %#v", expected, got)
	}
}

func TestRedisTopology_RedisNodesFromOptionsParsesCommaSeparated(t *testing.T) {
	ds := datasource.DataSource{
		Options: map[string]any{
			"nodes": "10.0.0.1:7000, 10.0.0.2:7001",
		},
	}
	expected := []string{"10.0.0.1:7000", "10.0.0.2:7001"}
	got := redisNodesFromOptions(ds)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("expected %#v, got %#v", expected, got)
	}
}

func TestRedisTopology_RedisProbeAddrFormatsIPv6Host(t *testing.T) {
	ds := datasource.DataSource{
		Host: "2001:db8::1",
		Port: 6379,
	}
	addr, err := redisProbeAddr(ds)
	if err != nil {
		t.Fatalf("redisProbeAddr: %v", err)
	}
	if addr != "[2001:db8::1]:6379" {
		t.Fatalf("expected bracketed ipv6 address, got %q", addr)
	}
}

func TestRedisTopology_RedisProbeAddrAcceptsBracketedIPv6Host(t *testing.T) {
	ds := datasource.DataSource{
		Host: "[2001:db8::1]",
		Port: 6379,
	}
	addr, err := redisProbeAddr(ds)
	if err != nil {
		t.Fatalf("redisProbeAddr: %v", err)
	}
	if addr != "[2001:db8::1]:6379" {
		t.Fatalf("expected normalized bracketed ipv6 address, got %q", addr)
	}
}
