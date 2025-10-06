-- tokenbucket.lua
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])
local now = tonumber(ARGV[4])
local ttl = tonumber(ARGV[5])

local state = redis.call('GET', key)
local tokens = capacity
local last_refill = now

if state then
    local decoded = cjson.decode(state)
    tokens = decoded.tokens
    last_refill = decoded.last_refill
end

if tokens < capacity then
    local delta = (now - last_refill) / 1000
    local tokens_to_add = delta * refill_rate
    if tokens_to_add > 0 then
        tokens = math.min(capacity, tokens + tokens_to_add)
        last_refill = now
    end
end

local allowed = false
if cost <= tokens then
    tokens = tokens - cost
    allowed = true
end

local new_state = cjson.encode({
    tokens = tokens,
    last_refill = last_refill,
    capacity = capacity,
    refill_rate = refill_rate
})

redis.call('SET', key, new_state, 'EX', ttl)
return {allowed and 1 or 0, math.floor(tokens)}