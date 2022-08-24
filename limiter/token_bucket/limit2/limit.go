package limiter

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
		-- 当前请求token数据
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
	// 存储容器
	store *redis.Client
	// lock
	rescueLock sync.Mutex
	// redis健康标识
	redisAlive uint32
	// redis故障时采用进程内 令牌桶限流器
	localLimiter map[string]*mrate.Limiter
	// localLimitLock
	rwMu sync.RWMutex
	// redis监控探测任务标识
	monitorStarted bool
	// 部署限流器机器个数，如果限流器降级到本地进程，则计算每台机器的限流速度和个数
	serverNum int
}

// NewTokenLimiter 初始函数
// limitServer -- 部署限流器机器个数，如果限流器降级到本地进程，则计算每台机器的限流速度和个数
func NewTokenLimiter(store *redis.Client, serverNum int) *TokenLimiter {

	return &TokenLimiter{
		store:        store,
		redisAlive:   1,
		localLimiter: make(map[string]*mrate.Limiter),
		serverNum:    serverNum,
	}
}

func (lim *TokenLimiter) Allow(requestKey string, rate, burst int) bool {
	return lim.AllowN(requestKey, rate, burst, time.Now(), 1)
}

func (lim *TokenLimiter) AllowN(requestKey string, rate, burst int, now time.Time, n int) bool {
	return lim.reserveN(requestKey, rate, burst, now, n)
}

func (lim *TokenLimiter) reserveN(requestKey string, rate, burst int, now time.Time, n int) bool {
	locLimiter := lim.getLimiter(requestKey, rate, burst)
	if atomic.LoadUint32(&lim.redisAlive) == 0 {
		return locLimiter.AllowN(now, n)
	}
	tokenKey := fmt.Sprintf(tokenFormat, requestKey)
	timestampKey := fmt.Sprintf(timestampFormat, requestKey)
	resp, err := lim.store.Eval(context.Background(),
		luaScript,
		[]string{
			tokenKey,
			timestampKey,
		},
		strconv.Itoa(rate),
		strconv.Itoa(burst),
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
		return locLimiter.AllowN(now, n)
	}

	code, ok := resp.(int64)
	if !ok {
		// todo 日志统一收集
		log.Printf("fail to eval redis script: %v, use in-process limiter for rescue", resp)
		lim.startMonitor()
		return locLimiter.AllowN(now, n)
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
			log.Println("redis recovery!")
			// 清空 localLimiter
			lim.cleanupLocalLimiter()
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

func (lim *TokenLimiter) getLimiter(key string, rate, burst int) *mrate.Limiter {
	if getV := lim.mapGetValue(key); getV != nil {
		return getV
	}

	// new a limiter
	value := lim.mapSetValue(key, rate, burst)
	return value
}

func (lim *TokenLimiter) mapGetValue(key string) *mrate.Limiter {
	lim.rwMu.RLock()
	defer lim.rwMu.RUnlock()
	if v, ok := lim.localLimiter[key]; ok {
		return v
	}
	return nil
}

func (lim *TokenLimiter) mapSetValue(key string, rate, burst int) *mrate.Limiter {
	lim.rwMu.Lock()
	defer lim.rwMu.Unlock()
	if v, ok := lim.localLimiter[key]; ok {
		return v
	}
	// 大概计算每台机器的限速
	localRate := rate / lim.serverNum
	if localRate <= 0 {
		localRate = 1
	}
	every := mrate.Every(time.Second / time.Duration(localRate))
	newLimiter := mrate.NewLimiter(every, burst)
	lim.localLimiter[key] = newLimiter
	return newLimiter
}

// 清空本地localLimiter
func (lim *TokenLimiter) cleanupLocalLimiter() {
	lim.rwMu.Lock()
	defer lim.rwMu.Unlock()
	lim.localLimiter = make(map[string]*mrate.Limiter)
}
