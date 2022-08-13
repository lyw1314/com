package main

import (
	"com/limiter/token_bucket/limit"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func main() {
	client := initRedis()
	limiter := limit.NewTokenLimiter(1, 100, client, "lyw", 2)

	r := gin.Default()
	r.GET("/hello", func(c *gin.Context) {
		allow := limiter.Allow()
		fmt.Println(allow)
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})
	r.Run(":8089")
}

func initRedis() *redis.Client {
	redisDb := redis.NewClient(&redis.Options{
		Addr:     "10.203.11.1:1806",
		Password: "004bab00bc2fa75",
		DB:       0,
	})
	return redisDb
}
