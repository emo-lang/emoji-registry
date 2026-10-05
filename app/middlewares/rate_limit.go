package middlewares

import (
	"fmt"
	"time"

	ratelimit "github.com/daqing/airway-ratelimit-plugin"
	"github.com/gin-gonic/gin"
)

// Registry rate limits. The auth rules share one backend so API and web
// attempts draw from the same per-IP budget while keeping their own 429
// response format.
var (
	publishLimiter = ratelimit.NewLimiter(30, time.Hour,
		ratelimit.WithKeyFunc(userOrIPKey),
		ratelimit.WithTooManyHandler(ratelimit.JSONTooMany("rate_limited", "too many requests, slow down")))

	authBackend = ratelimit.NewMemoryBackend()
	authAPI     = ratelimit.NewLimiter(10, time.Minute,
		ratelimit.WithBackend(authBackend),
		ratelimit.WithTooManyHandler(ratelimit.JSONTooMany("rate_limited", "too many requests, slow down")))
	authPage = ratelimit.NewLimiter(10, time.Minute,
		ratelimit.WithBackend(authBackend))

	searchLimiter = ratelimit.NewLimiter(60, time.Minute)
)

// LimitPublish throttles publish calls per authenticated user (30/hour).
// It must run after TokenAuth so current_user is set.
func LimitPublish() gin.HandlerFunc {
	return publishLimiter.Middleware()
}

// LimitAuthAPI throttles the JSON signup/login endpoints per IP (10/minute)
// with the shared error envelope.
func LimitAuthAPI() gin.HandlerFunc {
	return authAPI.Middleware()
}

// LimitAuthPage throttles the web signup/login forms per IP (10/minute) with
// a plain-text 429.
func LimitAuthPage() gin.HandlerFunc {
	return authPage.Middleware()
}

// LimitSearch throttles search requests per IP (60/minute) with a plain-text
// 429.
func LimitSearch() gin.HandlerFunc {
	return searchLimiter.Middleware()
}

func userOrIPKey(c *gin.Context) string {
	if user := CurrentUser(c); user != nil {
		return fmt.Sprintf("user:%d", user.ID)
	}
	return ratelimit.ByIP(c)
}
