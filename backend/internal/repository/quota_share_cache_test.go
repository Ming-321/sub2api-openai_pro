package repository

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestShouldResetQuotaShareWindowEnd(t *testing.T) {
	tests := []struct {
		name       string
		storedEnd  int64
		currentEnd int64
		tolerance  int64
		want       bool
	}{
		{name: "same end", storedEnd: 1000, currentEnd: 1000, tolerance: 120, want: false},
		{name: "small drift", storedEnd: 1000, currentEnd: 1008, tolerance: 120, want: false},
		{name: "large drift", storedEnd: 1000, currentEnd: 1400, tolerance: 120, want: true},
		{name: "missing stored", storedEnd: 0, currentEnd: 1400, tolerance: 120, want: true},
		{name: "missing current", storedEnd: 1000, currentEnd: 0, tolerance: 120, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldResetQuotaShareWindowEnd(tc.storedEnd, tc.currentEnd, tc.tolerance)
			if got != tc.want {
				t.Fatalf("shouldResetQuotaShareWindowEnd(%d, %d, %d) = %v, want %v", tc.storedEnd, tc.currentEnd, tc.tolerance, got, tc.want)
			}
		})
	}
}

func TestQuotaShareCacheResetLocalUSDDeletesOnlyTargetWindowKey(t *testing.T) {
	rdb := &quotaShareRedisFake{}
	cache := &quotaShareCache{rdb: rdb}

	if err := cache.ResetLocalUSD(context.Background(), 11, "5h"); err != nil {
		t.Fatalf("ResetLocalUSD returned error: %v", err)
	}

	want := "qs:lusd:11:5h"
	if len(rdb.deletedKeys) != 1 || rdb.deletedKeys[0] != want {
		t.Fatalf("deleted keys = %v, want [%s]", rdb.deletedKeys, want)
	}
}

type quotaShareRedisFake struct {
	deletedKeys []string
}

func (f *quotaShareRedisFake) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	f.deletedKeys = append(f.deletedKeys, keys...)
	return redis.NewIntResult(int64(len(keys)), nil)
}

func (f *quotaShareRedisFake) Pipeline() redis.Pipeliner {
	panic("not implemented")
}

func (f *quotaShareRedisFake) Get(ctx context.Context, key string) *redis.StringCmd {
	panic("not implemented")
}

func (f *quotaShareRedisFake) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	panic("not implemented")
}

func (f *quotaShareRedisFake) HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd {
	panic("not implemented")
}

func (f *quotaShareRedisFake) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	panic("not implemented")
}

func (f *quotaShareRedisFake) EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) *redis.Cmd {
	panic("not implemented")
}

func (f *quotaShareRedisFake) EvalRO(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	panic("not implemented")
}

func (f *quotaShareRedisFake) EvalShaRO(ctx context.Context, sha1 string, keys []string, args ...interface{}) *redis.Cmd {
	panic("not implemented")
}

func (f *quotaShareRedisFake) ScriptExists(ctx context.Context, hashes ...string) *redis.BoolSliceCmd {
	panic("not implemented")
}

func (f *quotaShareRedisFake) ScriptLoad(ctx context.Context, script string) *redis.StringCmd {
	panic("not implemented")
}
