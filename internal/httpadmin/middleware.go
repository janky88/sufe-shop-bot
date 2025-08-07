package httpadmin

import (
	"strconv"
	"time"
	
	"github.com/gin-gonic/gin"
	"shop-bot/internal/metrics"
	logger "shop-bot/internal/log"
	"shop-bot/pkg/middleware"
)

// requestLogger is a middleware that logs HTTP requests
func (s *Server) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get or generate trace ID
		traceID := c.GetHeader("X-Request-ID")
		if traceID == "" {
			traceID = c.GetHeader("X-Trace-ID")
		}
		if traceID == "" {
			traceID = middleware.GenerateTraceID()
		}
		
		// Add trace ID to context and response header
		c.Set("trace_id", traceID)
		c.Header("X-Trace-ID", traceID)
		
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		
		// Process request
		c.Next()
		
		// Log request details
		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		errorMsg := c.Errors.ByType(gin.ErrorTypePrivate).String()
		
		if raw != "" {
			path = path + "?" + raw
		}
		
		logger.Info("HTTP request",
			"trace_id", traceID,
			"client_ip", clientIP,
			"method", method,
			"path", path,
			"status", statusCode,
			"latency_ms", latency.Milliseconds(),
			"error", errorMsg,
		)
		
		// Record metrics
		metrics.HTTPRequestDuration.WithLabelValues(
			method,
			c.FullPath(),
			strconv.Itoa(statusCode),
		).Observe(latency.Seconds())
	}
}