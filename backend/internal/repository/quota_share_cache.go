package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	quotaShareGroupKeyPrefix    = "qs:g:"
	quotaShareUsageKeyPrefix    = "qs:u:"
	quotaShareWeightKey         = "qs:w:"
	quotaShareLocalUSDKeyPrefix = "qs:lusd:"
	quotaShareMinStateTTL       = 6 * time.Hour
	quotaShareWeightTTL         = 10 * time.Minute
	quotaShareWindowDriftTTL    = 2 * time.Minute
)

type quotaShareCache struct {
	rdb quotaShareRedisClient
}

type quotaShareRedisClient interface {
	redis.Scripter
	Pipeline() redis.Pipeliner
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

func NewQuotaShareCache(rdb redis.Cmdable) service.QuotaShareCache {
	return &quotaShareCache{rdb: rdb}
}

func groupStateKey(groupID int64) string {
	return fmt.Sprintf("%s%d", quotaShareGroupKeyPrefix, groupID)
}

func keyUsageKey(groupID, keyID int64) string {
	return fmt.Sprintf("%s%d:%d", quotaShareUsageKeyPrefix, groupID, keyID)
}

func totalWeightKey(groupID int64) string {
	return fmt.Sprintf("%s%d", quotaShareWeightKey, groupID)
}

func localUSDKey(groupID int64, window string) string {
	return fmt.Sprintf("%s%d:%s", quotaShareLocalUSDKeyPrefix, groupID, window)
}

func (c *quotaShareCache) GetGroupState(ctx context.Context, groupID int64) (*service.QuotaShareGroupState, error) {
	data, err := c.rdb.Get(ctx, groupStateKey(groupID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state service.QuotaShareGroupState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (c *quotaShareCache) SetGroupState(ctx context.Context, groupID int64, state *service.QuotaShareGroupState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	ttl := c.computeStateTTL(state)
	return c.rdb.Set(ctx, groupStateKey(groupID), data, ttl).Err()
}

// computeStateTTL returns max(window5hEnd, window7dEnd) - now + 1h buffer,
// floored to quotaShareMinStateTTL so that estimate-only states also survive.
func (c *quotaShareCache) computeStateTTL(state *service.QuotaShareGroupState) time.Duration {
	now := time.Now().Unix()
	maxEnd := state.Window5hEnd
	if state.Window7dEnd > maxEnd {
		maxEnd = state.Window7dEnd
	}
	if maxEnd > now {
		ttl := time.Duration(maxEnd-now)*time.Second + time.Hour
		if ttl < quotaShareMinStateTTL {
			return quotaShareMinStateTTL
		}
		return ttl
	}
	return quotaShareMinStateTTL
}

func (c *quotaShareCache) GetKeyUsage(ctx context.Context, groupID, keyID int64) (*service.QuotaShareKeyUsage, error) {
	vals, err := c.rdb.HGetAll(ctx, keyUsageKey(groupID, keyID)).Result()
	if err != nil {
		return nil, err
	}
	if len(vals) == 0 {
		return nil, nil
	}

	usage := &service.QuotaShareKeyUsage{}
	if v, ok := vals["u5"]; ok {
		usage.Usage5h, _ = strconv.ParseFloat(v, 64)
	}
	if v, ok := vals["u7"]; ok {
		usage.Usage7d, _ = strconv.ParseFloat(v, 64)
	}
	return usage, nil
}

func shouldResetQuotaShareWindowEnd(storedEnd, currentEnd int64, toleranceSeconds int64) bool {
	if storedEnd == currentEnd {
		return false
	}
	if storedEnd == 0 || currentEnd == 0 {
		return true
	}
	diff := storedEnd - currentEnd
	if diff < 0 {
		diff = -diff
	}
	return diff > toleranceSeconds
}

// incrKeyUsageLua atomically increments usage, resetting only when the stored
// window end is clearly different from the current fixed global window. Codex
// reset-after headers can drift by a few seconds inside the same upstream window.
var incrKeyUsageLua = redis.NewScript(`
local key = KEYS[1]
local cost = tonumber(ARGV[1])
local w5end = ARGV[2]
local w7end = ARGV[3]
local drift = tonumber(ARGV[4]) or 120

local stored_w5 = redis.call('HGET', key, 'w5e')
local stored_w7 = redis.call('HGET', key, 'w7e')

local function should_reset(stored, current)
    if stored == false or stored == nil then
        return true
    end
    if stored == current then
        return false
    end
    local stored_num = tonumber(stored)
    local current_num = tonumber(current)
    if stored_num == nil or current_num == nil then
        return true
    end
    if stored_num == 0 or current_num == 0 then
        return true
    end
    return math.abs(stored_num - current_num) > drift
end

-- Reset 5h usage if window clearly changed
if should_reset(stored_w5, w5end) then
    redis.call('HSET', key, 'u5', tostring(cost), 'w5e', w5end)
else
    redis.call('HINCRBYFLOAT', key, 'u5', cost)
    redis.call('HSET', key, 'w5e', w5end)
end

-- Reset 7d usage if window clearly changed
if should_reset(stored_w7, w7end) then
    redis.call('HSET', key, 'u7', tostring(cost), 'w7e', w7end)
else
    redis.call('HINCRBYFLOAT', key, 'u7', cost)
    redis.call('HSET', key, 'w7e', w7end)
end

-- Set expiry to max of both windows + buffer
redis.call('EXPIRE', key, 604800 + 3600)

local u5 = tonumber(redis.call('HGET', key, 'u5')) or 0
local u7 = tonumber(redis.call('HGET', key, 'u7')) or 0
return {tostring(u5), tostring(u7)}
`)

func (c *quotaShareCache) IncrKeyUsage(ctx context.Context, groupID, keyID int64, cost float64, window5hEnd, window7dEnd int64) (*service.QuotaShareKeyUsage, error) {
	result, err := incrKeyUsageLua.Run(ctx, c.rdb,
		[]string{keyUsageKey(groupID, keyID)},
		cost,
		strconv.FormatInt(window5hEnd, 10),
		strconv.FormatInt(window7dEnd, 10),
		strconv.FormatInt(int64(quotaShareWindowDriftTTL/time.Second), 10),
	).StringSlice()
	if err != nil {
		return nil, err
	}

	usage := &service.QuotaShareKeyUsage{}
	if len(result) >= 2 {
		usage.Usage5h, _ = strconv.ParseFloat(result[0], 64)
		usage.Usage7d, _ = strconv.ParseFloat(result[1], 64)
	}
	return usage, nil
}

func (c *quotaShareCache) IncrLocalUSD(ctx context.Context, groupID int64, window string, cost float64) error {
	key := localUSDKey(groupID, window)
	pipe := c.rdb.Pipeline()
	pipe.IncrByFloat(ctx, key, cost)
	pipe.Expire(ctx, key, 8*24*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

var getAndResetLocalUSDLua = redis.NewScript(`
local key = KEYS[1]
local val = redis.call('GET', key)
if val then
    redis.call('DEL', key)
    return val
end
return "0"
`)

func (c *quotaShareCache) GetAndResetLocalUSD(ctx context.Context, groupID int64, window string) (float64, error) {
	result, err := getAndResetLocalUSDLua.Run(ctx, c.rdb, []string{localUSDKey(groupID, window)}).Text()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(result, 64)
}

func (c *quotaShareCache) ResetLocalUSD(ctx context.Context, groupID int64, window string) error {
	return c.rdb.Del(ctx, localUSDKey(groupID, window)).Err()
}

func (c *quotaShareCache) GetTotalWeight(ctx context.Context, groupID int64) (int, error) {
	val, err := c.rdb.Get(ctx, totalWeightKey(groupID)).Int()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

func (c *quotaShareCache) SetTotalWeight(ctx context.Context, groupID int64, total int) error {
	return c.rdb.Set(ctx, totalWeightKey(groupID), total, quotaShareWeightTTL).Err()
}

func (c *quotaShareCache) DeleteTotalWeight(ctx context.Context, groupID int64) error {
	return c.rdb.Del(ctx, totalWeightKey(groupID)).Err()
}
