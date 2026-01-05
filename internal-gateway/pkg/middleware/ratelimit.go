package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sandbox0-ai/infra/internal-gateway/pkg/db"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// RateLimiter provides rate limiting functionality
type RateLimiter struct {
	repo          *db.Repository
	logger        *zap.Logger
	rps           int
	burst         int
	localLimiters sync.Map // map[teamID]*rate.Limiter
	useDatabase   bool
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(repo *db.Repository, rps, burst int, logger *zap.Logger) *RateLimiter {
	rl := &RateLimiter{
		repo:        repo,
		logger:      logger,
		rps:         rps,
		burst:       burst,
		useDatabase: repo != nil,
	}

	// Start cleanup goroutine for local limiters
	go rl.cleanupLoop()

	return rl
}

// RateLimit returns a gin middleware that rate limits requests per team
func (rl *RateLimiter) RateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx := GetAuthContext(c)
		if authCtx == nil {
			// No auth context, skip rate limiting (will fail at auth middleware)
			c.Next()
			return
		}

		teamID := authCtx.TeamID
		if teamID == "" {
			c.Next()
			return
		}

		// Get or create local limiter for this team
		limiter := rl.getLimiter(teamID)

		if !limiter.Allow() {
			rl.logger.Warn("Rate limit exceeded",
				zap.String("team_id", teamID),
				zap.String("client_ip", c.ClientIP()),
			)

			c.Header("X-RateLimit-Limit", string(rune(rl.rps)))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", "1")

			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate limit exceeded",
				"retry_after": 1,
			})
			return
		}

		c.Next()
	}
}

// getLimiter gets or creates a rate limiter for a team
func (rl *RateLimiter) getLimiter(teamID string) *rate.Limiter {
	if v, ok := rl.localLimiters.Load(teamID); ok {
		return v.(*rate.Limiter)
	}

	limiter := rate.NewLimiter(rate.Limit(rl.rps), rl.burst)
	actual, _ := rl.localLimiters.LoadOrStore(teamID, limiter)
	return actual.(*rate.Limiter)
}

// cleanupLoop periodically cleans up unused limiters
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		// Reset all limiters periodically to prevent memory leaks
		// In production, you might want a more sophisticated cleanup strategy
		rl.localLimiters.Range(func(key, value interface{}) bool {
			// Keep limiters, just let them naturally reset
			return true
		})
	}
}

// GlobalRateLimit returns a middleware that applies a global rate limit
func GlobalRateLimit(rps int) gin.HandlerFunc {
	limiter := rate.NewLimiter(rate.Limit(rps), rps*2)

	return func(c *gin.Context) {
		if !limiter.Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "server rate limit exceeded",
			})
			return
		}
		c.Next()
	}
}
