package storage

import (
	"strings"
	"testing"

	"live-auction/backend/internal/config"
)

func TestOpenRedisSelectsSentinelClient(t *testing.T) {
	client, err := openRedis(config.Config{
		RedisMode:               "sentinel",
		RedisSentinelMasterName: "mymaster",
		RedisSentinelAddrs:      "127.0.0.1:26379, 127.0.0.1:26380",
		RedisPassword:           "redis-pass",
		RedisDB:                 2,
		RedisPoolSize:           8,
		RedisMinIdleConns:       2,
	})
	if err != nil {
		t.Fatalf("openRedis sentinel: %v", err)
	}
	defer func() { _ = client.Close() }()

	opts := client.Options()
	if opts.DB != 2 || opts.Password != "redis-pass" || opts.PoolSize != 8 || opts.MinIdleConns != 2 {
		t.Fatalf("redis options = %#v", opts)
	}
}

func TestOpenRedisRejectsClusterModeUntilKeyTopologyIsFixed(t *testing.T) {
	_, err := openRedis(config.Config{RedisMode: "cluster"})
	if err == nil {
		t.Fatal("openRedis cluster returned nil error")
	}
	if !strings.Contains(err.Error(), "not supported") || !strings.Contains(err.Error(), "Lua key topology") {
		t.Fatalf("cluster error = %q", err.Error())
	}
}

func TestOpenRedisSentinelRequiresDiscoveryConfig(t *testing.T) {
	_, err := openRedis(config.Config{RedisMode: "sentinel"})
	if err == nil {
		t.Fatal("openRedis sentinel without discovery config returned nil error")
	}
	if !strings.Contains(err.Error(), "REDIS_SENTINEL_MASTER_NAME") {
		t.Fatalf("sentinel error = %q", err.Error())
	}
}
