package main

import (
	"api_module/datasource"
	"api_module/pkg/limiter"
	"api_module/pkg/setting"
	"fmt"
	"github.com/go-redis/redis/v8"
	"time"
)

var redisClient *redis.Client

func init() {
	redConf := &setting.Redis{
		Host:           "127.0.0.1:6379",
		Password:       "",
		MaxIdle:        30,
		MaxActive:      30,
		IdleTimeout:    200,
		MinIdleCons:    100,
		PoolSize:       5,
		ConnectTimeout: 30,
		ReadTimeout:    3,
		WriteTimeout:   3,
	}
	redisClient = datasource.IniRedis(redConf)
}
func main() {
	tokenLimiter := limiter.NewTokenLimiter(redisClient, 2)
	for i := 0; i < 10000; i++ {
		allow := tokenLimiter.Allow("aaa", 1, 2)
		if allow {
			fmt.Println("allow")
		} else {
			fmt.Println("forbiden")
		}
		time.Sleep(time.Millisecond * 100)
	}
}
