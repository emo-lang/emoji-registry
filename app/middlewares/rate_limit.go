package middlewares

import (
	"fmt"
	"sync"
	"time"

	ratelimit "github.com/daqing/airway-ratelimit-plugin"
	airwayredis "github.com/daqing/airway-redis-plugin"
	"github.com/gin-gonic/gin"
)

// Registry rate limits. Limiters are built lazily on first use — by then the
// server boot has set up Redis, so a configured Redis switches every rule to
// the Redis backend (one shared instance: keys carry the window length, so
// budgets never collide, and the API/web auth rules still share one budget).
var (
	limitersOnce   sync.Once
	publishLimiter *ratelimit.Limiter
	authAPI        *ratelimit.Limiter
	authPage       *ratelimit.Limiter
	searchLimiter  *ratelimit.Limiter
)

func buildLimiters() {
	json429 := ratelimit.WithTooManyHandler(ratelimit.JSONTooMany("rate_limited", "too many requests, slow down"))

	if rdb := airwayredis.Current(); rdb != nil {
		backend := ratelimit.NewRedisBackend(rdb, "ratelimit")

		publishLimiter = ratelimit.NewLimiter(30, time.Hour,
			ratelimit.WithKeyFunc(userOrIPKey), ratelimit.WithBackend(backend), json429)
		authAPI = ratelimit.NewLimiter(10, time.Minute,
			ratelimit.WithBackend(backend), ratelimit.WithKeyFunc(authIPKey), json429)
		authPage = ratelimit.NewLimiter(10, time.Minute,
			ratelimit.WithBackend(backend), ratelimit.WithKeyFunc(authIPKey))
		searchLimiter = ratelimit.NewLimiter(60, time.Minute,
			ratelimit.WithBackend(backend), ratelimit.WithKeyFunc(searchIPKey))
		return
	}

	publishLimiter = ratelimit.NewLimiter(30, time.Hour,
		ratelimit.WithKeyFunc(userOrIPKey), json429)

	authBackend := ratelimit.NewMemoryBackend()
	authAPI = ratelimit.NewLimiter(10, time.Minute,
		ratelimit.WithBackend(authBackend), ratelimit.WithKeyFunc(authIPKey), json429)
	authPage = ratelimit.NewLimiter(10, time.Minute,
		ratelimit.WithBackend(authBackend), ratelimit.WithKeyFunc(authIPKey))

	searchLimiter = ratelimit.NewLimiter(60, time.Minute, ratelimit.WithKeyFunc(searchIPKey))
}

// RateLimitBackendName reports the active counter store: "redis" or "memory".
func RateLimitBackendName() string {
	if airwayredis.Current() != nil {
		return "redis"
	}
	return "memory"
}

// LimitPublish throttles publish calls per authenticated user (30/hour).
// It must run after TokenAuth so current_user is set.
func LimitPublish() gin.HandlerFunc {
	limitersOnce.Do(buildLimiters)
	return publishLimiter.Middleware()
}

// LimitAuthAPI throttles the JSON signup/login endpoints per IP (10/minute)
// with the shared error envelope.
func LimitAuthAPI() gin.HandlerFunc {
	limitersOnce.Do(buildLimiters)
	return authAPI.Middleware()
}

// LimitAuthPage throttles the web signup/login forms per IP (10/minute) with
// a plain-text 429.
func LimitAuthPage() gin.HandlerFunc {
	limitersOnce.Do(buildLimiters)
	return authPage.Middleware()
}

// LimitSearch throttles search requests per IP (60/minute) with a plain-text
// 429.
func LimitSearch() gin.HandlerFunc {
	limitersOnce.Do(buildLimiters)
	return searchLimiter.Middleware()
}

func userOrIPKey(c *gin.Context) string {
	if user := CurrentUser(c); user != nil {
		return fmt.Sprintf("publish:user:%d", user.ID)
	}
	return "publish:" + ratelimit.ByIP(c)
}

// authIPKey and searchIPKey namespace the keys so rules that share a window
// length (both 60s) do not share a counter on the Redis backend.
func authIPKey(c *gin.Context) string {
	return "auth:" + ratelimit.ByIP(c)
}

func searchIPKey(c *gin.Context) string {
	return "search:" + ratelimit.ByIP(c)
}
