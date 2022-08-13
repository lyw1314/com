// 分布式限流器
// 滑动窗口算法实现
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

var redisDb *redis.Client
var luaScript = `
	local key = KEYS[1];
	local now_time = ARGV[1];
	local period = ARGV[2];
	local requests = ARGV[3];

	local before_time = now_time - period*1000000000;
	
	redis.call("ZREMRANGEBYSCORE",key,0,before_time);
	if redis.call("ZCARD",key) >= tonumber(requests) then
		return 0
	end
	redis.call("ZADD",key,now_time,now_time);
	redis.call("EXPIRE",key,period);
	return 1
`
var evalSha string
var sucNum int

func init() {
	initRedis()
}
func main() {
	r := gin.Default()
	r.GET("/hello", func(c *gin.Context) {
		doRequest()
		log.Println(time.Now().UnixNano())
		c.JSON(http.StatusOK, gin.H{"data": "ok"})
	})
	r.Run(":8088")
}

func initRedis() {
	redisDb = redis.NewClient(&redis.Options{
		Addr:     "10.203.11.1:1806",
		Password: "004bab00bc2fa75",
		DB:       0,
	})
	var err error
	evalSha, err = redisDb.ScriptLoad(context.Background(), luaScript).Result()
	if err != nil {
		log.Fatal(err)
	}
}

func isAllow(uid, action string, period, maxCount int) bool {
	key := fmt.Sprintf("%v_%v", uid, action)
	now := time.Now().UnixNano() //纳秒
	res, err := redisDb.EvalSha(context.Background(), evalSha, []string{key}, now, period, maxCount).Result()
	if err != nil {
		log.Println("err:", err)
	}
	log.Println("===", res)
	if res.(int64) == int64(0) {
		return false
	}
	return true
}

func doRequest() {
	ret := isAllow("lyw", "add", 5, 10)
	if ret {
		sucNum++
		log.Println("成功：", sucNum)
	} else {
		log.Println("失败")
	}
}
