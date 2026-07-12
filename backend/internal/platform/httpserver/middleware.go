package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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
		if !validRequestID(requestID) {
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

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, character := range value {
		valid := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '.' || character == '_' || character == '~'
		if !valid {
			return false
		}
	}
	return true
}

func recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		original := ctx.Writer
		transaction := newTransactionWriter(original)
		ctx.Writer = transaction
		defer func() {
			ctx.Writer = original
			if recovered := recover(); recovered != nil {
				attrs := append(observability.ContextAttrs(ctx.Request.Context()), slog.String(methodKey, ctx.Request.Method), slog.String(pathKey, ctx.Request.URL.Path))
				logger.LogAttrs(ctx.Request.Context(), slog.LevelError, "http.panic_recovered", attrs...)
				ctx.Abort()
				if transaction.reset() {
					ctx.Writer = transaction
					ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
					if err := transaction.commit(); err != nil {
						logger.LogAttrs(ctx.Request.Context(), slog.LevelError, "http.response_commit_failed", observability.Error(err))
					}
					ctx.Writer = original
					return
				}
				if closed, closeErr := transaction.closeHijacked(); closed {
					if closeErr != nil {
						logger.LogAttrs(ctx.Request.Context(), slog.LevelError, "http.hijacked_close_failed", observability.Error(closeErr))
					}
					return
				}
				panic(http.ErrAbortHandler)
			}
			if err := transaction.commit(); err != nil {
				logger.LogAttrs(ctx.Request.Context(), slog.LevelError, "http.response_commit_failed", observability.Error(err))
			}
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

// requestLimit 按精确方法与路径选择流式请求上限，其他请求保持安全默认值。
func requestLimit(defaultLimit, uploadLimit int64) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		limit := defaultLimit
		if ctx.Request.Method == http.MethodPost && ctx.Request.URL.Path == "/api/v1/article-images" && ctx.Request.URL.RawQuery == "" && !ctx.Request.URL.ForceQuery {
			limit = uploadLimit
		}
		if ctx.Request.Body == nil {
			ctx.Next()
			return
		}
		if ctx.Request.ContentLength > limit {
			ctx.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request_too_large"})
			return
		}
		limited := http.MaxBytesReader(ctx.Writer, ctx.Request.Body, limit)
		ctx.Request.Body = &requestBody{ReadCloser: limited, context: ctx}
		ctx.Next()
	}
}

// requestBody 在读取超过限制时立即中止请求并保持统一 JSON 错误。
type requestBody struct {
	// ReadCloser 是标准库提供的流式限制读取器。
	io.ReadCloser
	// context 用于在读取越界时终止当前 Gin 请求。
	context *gin.Context
}

// Read 转发流式读取，并把 MaxBytesError 映射为稳定的 413 JSON 响应。
func (body *requestBody) Read(buffer []byte) (int, error) {
	read, err := body.ReadCloser.Read(buffer)
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) && !body.context.IsAborted() {
		body.context.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request_too_large"})
	}
	return read, err
}
