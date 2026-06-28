local key = KEYS[1]
local limit = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local request_id = ARGV[3]

local redis_time = redis.call("TIME")
local now_ms = (tonumber(redis_time[1]) * 1000) + math.floor(tonumber(redis_time[2]) / 1000)
local window_start = now_ms - window_ms

redis.call("ZREMRANGEBYSCORE", key, "-inf", window_start)

local count = redis.call("ZCARD", key)
local allowed = 0

if count < limit then
    redis.call("ZADD", key, now_ms, request_id)
    count = count + 1
    allowed = 1
end

redis.call("PEXPIRE", key, window_ms)

local remaining = limit - count
if remaining < 0 then
    remaining = 0
end

local reset_ms = now_ms + window_ms
local oldest = redis.call("ZRANGE", key, 0, 0, "WITHSCORES")
if #oldest == 2 then
    reset_ms = tonumber(oldest[2]) + window_ms
end

return {allowed, limit, remaining, reset_ms, now_ms}
