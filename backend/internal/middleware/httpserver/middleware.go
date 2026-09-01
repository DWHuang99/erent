// Package httpserver contains transport-wide Gin middleware.
package httpserver

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
)

const RequestIDHeader = "X-Request-Id"

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(RequestIDHeader)
		if !validRequestID.MatchString(requestID) {
			var err error
			requestID, err = newRequestID()
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "failed to create request id"})
				return
			}
		}
		c.Header(RequestIDHeader, requestID)
		c.Set("requestID", requestID)
		c.Next()
	}
}

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered", "error", recovered, "request_id", c.GetString("requestID"), "stack", string(debug.Stack()))
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "internal server error"})
			}
		}()
		c.Next()
	}
}

func AccessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()
		logger.Info("http request completed",
			"bytes", c.Writer.Size(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"request_id", c.GetString("requestID"),
			"status", c.Writer.Status(),
		)
	}
}

func IsValidRequestID(value string) bool {
	return validRequestID.MatchString(value)
}

func newRequestID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
