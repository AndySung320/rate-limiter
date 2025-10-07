local user_key = KEYS[1]
local global_key = KEYS[2]

local global_capacity = tonumber(ARGV[1])
local global_refill_rate = tonumber(ARGV[2])
local user_capacity = tonumber(ARGV[3])
local user_refill_rate = tonumber(ARGV[4])
local cost = tonumber(ARGV[5])
local now = tonumber(ARGV[6])
local ttl = tonumber(ARGV[7])

-- Initialize default state
local user_tokens = user_capacity
local user_last_refill = now
local global_tokens = global_capacity
local global_last_refill = now

-- Read user bucket state from Redis
local user_state = redis.call('GET', user_key)
if user_state then
    local decoded = cjson.decode(user_state)
    user_tokens = decoded.user_tokens
    user_last_refill = decoded.user_last_refill
end

-- Read global bucket state from Redis
local global_state = redis.call('GET', global_key)
if global_state then
    local decoded = cjson.decode(global_state)
    global_tokens = decoded.global_tokens
    global_last_refill = decoded.global_last_refill
end

-- Refill user tokens based on elapsed time
if user_tokens < user_capacity then
    local delta = (now - user_last_refill) / 1000
    local tokens_to_add = delta * user_refill_rate
    if tokens_to_add > 0 then
        user_tokens = math.min(user_capacity, user_tokens + tokens_to_add)
        user_last_refill = now
    end
end

-- Refill global tokens based on elapsed timeif global_tokens < global_capacity then
if global_tokens < global_capacity then
    local delta = (now - global_last_refill) / 1000
    local tokens_to_add = delta * global_refill_rate
    if tokens_to_add > 0 then
        global_tokens = math.min(global_capacity, global_tokens + tokens_to_add)
        global_last_refill = now
    end
end

-- Check both user and global buckets for availability
local allowed = false
if cost <= user_tokens and cost <= global_tokens then
    user_tokens = user_tokens - cost
    global_tokens = global_tokens - cost
    allowed = true
end

-- Save updated user state
local user_new_state = cjson.encode({
    user_tokens = user_tokens,
    user_last_refill = user_last_refill,
    user_capacity = user_capacity,
    user_refill_rate = user_refill_rate
})

-- Save updated global state
local global_new_state = cjson.encode({
    global_tokens = global_tokens,
    global_last_refill = global_last_refill,
    global_capacity = global_capacity,
    global_refill_rate = global_refill_rate
})

redis.call('SET', user_key, user_new_state, 'EX', ttl)
redis.call('SET', global_key, global_new_state, 'EX', ttl)

-- Return: [allowed (1/0), remaining user tokens, remaining global tokens]
return {allowed and 1 or 0, math.floor(user_tokens), math.floor(global_tokens)}