package limit

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	mrate "golang.org/x/time/rate"
)

const (
	luaScript = `
		-- 每秒生产token数量
		local rate = tonumber(ARGV[1])
		-- 桶容量
		local capacity = tonumber(ARGV[2])
		-- 当前时间戳
		local now = tonumber(ARGV[3])
		-- 一次请求token的个数
		local requested = tonumber(ARGV[4])
		-- 需要多少秒才能填满桶
		local fill_time = capacity/rate
		-- 向下取整,ttl为填满时间的2倍
		local ttl = math.floor(fill_time*2)
		-- 当前时间剩余桶容量
		local last_tokens = tonumber(redis.call("get", KEYS[1]))
		
		-- 如果当前桶容量为0,说明是第一次进入,则默认容量为桶的最大容量
		if last_tokens == nil then
			last_tokens = capacity
		end
		
		-- 上一次的请求时间
		local last_request_time = tonumber(redis.call("get", KEYS[2]))
		
		-- 如果是第一次进入则设置上一次请求时间为0
		if last_request_time == nil then
			last_request_time = 0
		end
		
		-- 距离上次请求的时间跨度
		local delta = math.max(0, now-last_request_time)
		
		-- 计算上次请求到现在总共新产生的token的数量,然后跟桶容量取最小值，就是当前令牌数
		local filled_tokens = math.min(capacity, last_tokens+(delta*rate))

		-- 桶剩余数量
		local new_tokens = filled_tokens

		-- 本次请求token数量是否足够
		local allowed = filled_tokens >= requested
		
		-- 如果token足够本次请求,则进行扣减，计算剩余数量
		if allowed then
			new_tokens = filled_tokens - requested
		end
		
		-- 设置剩余token数量
		redis.call("setex", KEYS[1], ttl, new_tokens)

		-- 刷新上一次提交时间为本次提交
		redis.call("setex", KEYS[2], ttl, now)
		
		return allowed
	`
	tokenFormat     = "%s_tokens"
	timestampFormat = "%s_ts"
	pingInterval    = time.Millisecond * 100
)

type TokenLimiter struct {
	// 每秒生产速率
	rate int
	// 桶容量
	burst int
	// 存储容器
	store *redis.Client
	// redis key，记录桶内令牌剩余容量
	tokenKey string
	// 记录上一次请求时间的key
	timestampKey string
	// lock
	rescueLock sync.Mutex
	// redis健康标识
	redisAlive uint32
	// redis故障时采用进程内 令牌桶限流器
	rescueLimiter *mrate.Limiter
	// 部署限流器的机器个数，如果限流器降级到本地进程，则计算每台机器的限流速度和个数
	localServerNum int
	// redis监控探测任务标识
	monitorStarted bool
}

// NewTokenLimiter 初始函数
// limitServer -- 部署限流器机器个数，如果限流器降级到本地进程，则计算每台机器的限流速度和个数
func NewTokenLimiter(rate, burst int, store *redis.Client, key string, localServerNum int) *TokenLimiter {
	tokenKey := fmt.Sprintf(tokenFormat, key)
	timestampKey := fmt.Sprintf(timestampFormat, key)

	rageB := burst
	if burst > localServerNum {
		rageB = burst / localServerNum
	}

	return &TokenLimiter{
		rate:          rate,
		burst:         burst,
		store:         store,
		tokenKey:      tokenKey,
		timestampKey:  timestampKey,
		redisAlive:    1,
		rescueLimiter: mrate.NewLimiter(mrate.Every(time.Second/time.Duration(rate)), rageB),
	}
}

func (lim *TokenLimiter) Allow() bool {
	return lim.AllowN(time.Now(), 1)
}

func (lim *TokenLimiter) AllowN(now time.Time, n int) bool {
	return lim.reserveN(now, n)
}

func (lim *TokenLimiter) reserveN(now time.Time, n int) bool {
	if atomic.LoadUint32(&lim.redisAlive) == 0 {
		return lim.rescueLimiter.AllowN(now, n)
	}
	resp, err := lim.store.Eval(context.Background(),
		luaScript,
		[]string{
			lim.tokenKey,
			lim.timestampKey,
		},
		strconv.Itoa(lim.rate),
		strconv.Itoa(lim.burst),
		strconv.FormatInt(now.Unix(), 10),
		strconv.Itoa(n),
	).Result()
	// redis allowed == false
	// Lua boolean false -> r Nil bulk reply
	if err == redis.Nil {
		return false
	}
	if err != nil {
		// todo 日志统一收集
		log.Printf("fail to use rate limiter: %s, use in-process limiter for rescue", err)
		lim.startMonitor()
		return lim.rescueLimiter.AllowN(now, n)
	}

	code, ok := resp.(int64)
	if !ok {
		// todo 日志统一收集
		log.Printf("fail to eval redis script: %v, use in-process limiter for rescue", resp)
		lim.startMonitor()
		return lim.rescueLimiter.AllowN(now, n)
	}

	// redis allowed == true
	// Lua boolean true -> r integer reply with value of 1
	return code == 1
}

func (lim *TokenLimiter) startMonitor() {
	lim.rescueLock.Lock()
	defer lim.rescueLock.Unlock()

	if lim.monitorStarted {
		return
	}

	lim.monitorStarted = true
	atomic.StoreUint32(&lim.redisAlive, 0)

	go lim.waitForRedis()
}

func (lim *TokenLimiter) waitForRedis() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		lim.rescueLock.Lock()
		lim.monitorStarted = false
		lim.rescueLock.Unlock()
	}()

	for range ticker.C {
		if lim.ping() {
			atomic.StoreUint32(&lim.redisAlive, 1)
			return
		}
	}
}

func (lim *TokenLimiter) ping() bool {
	result, err := lim.store.Ping(context.Background()).Result()
	if err != nil {
		return false
	}
	return result == "PONG"
}
