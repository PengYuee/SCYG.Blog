package httpserver

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
)

const (
	methodKey   = "method"
	pathKey     = "path"
	statusKey   = "status"
	durationKey = "duration"
)

func requestID() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		requestID := ctx.GetHeader("X-Request-ID")
		if requestID == "" {
			raw := make([]byte, 16)
			if _, err := rand.Read(raw); err != nil {
				ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
				return
			}
			requestID = hex.EncodeToString(raw)
		}
		requestContext := observability.WithRequestFields(ctx.Request.Context(), observability.RequestFields{RequestID: requestID})
		ctx.Request = ctx.Request.WithContext(requestContext)
		ctx.Header("X-Request-ID", requestID)
		ctx.Next()
	}
}

func recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		defer func() {
			if recover() == nil {
				return
			}
			attrs := append(observability.ContextAttrs(ctx.Request.Context()), slog.String(methodKey, ctx.Request.Method), slog.String(pathKey, ctx.Request.URL.Path))
			logger.LogAttrs(ctx.Request.Context(), slog.LevelError, "http.panic_recovered", attrs...)
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		}()
		ctx.Next()
	}
}

func accessLog(logger *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		started := time.Now()
		ctx.Next()
		attrs := append(observability.ContextAttrs(ctx.Request.Context()),
			slog.String(methodKey, ctx.Request.Method), slog.String(pathKey, ctx.Request.URL.Path),
			slog.Int(statusKey, ctx.Writer.Status()), slog.Duration(durationKey, time.Since(started)))
		logger.LogAttrs(ctx.Request.Context(), slog.LevelInfo, "http.request", attrs...)
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		ctx.Header("X-Content-Type-Options", "nosniff")
		ctx.Header("X-Frame-Options", "DENY")
		ctx.Header("Referrer-Policy", "no-referrer")
		ctx.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:")
		ctx.Next()
	}
}

func cors(origins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[origin] = struct{}{}
	}
	return func(ctx *gin.Context) {
		origin := ctx.GetHeader("Origin")
		if _, ok := allowed[origin]; ok {
			ctx.Header("Access-Control-Allow-Origin", origin)
			ctx.Header("Vary", "Origin")
			ctx.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			ctx.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, If-Match, X-Request-ID")
		}
		if ctx.Request.Method == http.MethodOptions {
			if _, ok := allowed[origin]; ok {
				ctx.AbortWithStatus(http.StatusNoContent)
				return
			}
			ctx.AbortWithStatus(http.StatusForbidden)
			return
		}
		ctx.Next()
	}
}

func requestLimit(limit int64) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if ctx.Request.Body == nil {
			ctx.Next()
			return
		}
		if ctx.Request.ContentLength > limit {
			ctx.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request_too_large"})
			return
		}
		body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, limit+1))
		closeErr := ctx.Request.Body.Close()
		if err != nil || closeErr != nil {
			ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid_request_body"})
			return
		}
		if int64(len(body)) > limit {
			ctx.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request_too_large"})
			return
		}
		ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
		ctx.Request.ContentLength = int64(len(body))
		ctx.Next()
	}
}
