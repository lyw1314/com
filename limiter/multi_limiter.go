package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
	"net/http"
	"sync"
)

type multiLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	b        int
}

var ml *multiLimiter

func init() {
	ml = NewMultiLimiter(50, 1)
}

func main() {
	r := gin.Default()
	r.GET("/hello", hello)

	r.Run(":8081")
}

func hello(c *gin.Context) {
	key := c.Query("key")
	limiter := ml.getLimiter(key)
	allow := limiter.Allow()
	if allow {
		c.JSON(http.StatusOK, gin.H{"data": "success"})
		return
	}
	fmt.Println("===", ml.limiters)
	c.JSON(http.StatusOK, gin.H{"data": "fail"})
}

func NewMultiLimiter(r rate.Limit, b int) *multiLimiter {
	return &multiLimiter{
		limiters: make(map[string]*rate.Limiter),
		r:        r,
		b:        b,
	}
}

func (m *multiLimiter) getLimiter(key string) *rate.Limiter {
	if getV := m.mapGetValue(key); getV != nil {
		return getV
	}

	// new a limiter
	value := m.mapSetValue(key)
	return value
}

func (m *multiLimiter) mapGetValue(key string) *rate.Limiter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if v, ok := m.limiters[key]; ok {
		return v
	}
	return nil
}

func (m *multiLimiter) mapSetValue(key string) *rate.Limiter {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.limiters[key]; ok {
		return v
	}
	newLimiter := rate.NewLimiter(m.r, m.b)
	m.limiters[key] = newLimiter
	return newLimiter
}
