// Package contract enforces the generated OpenAPI contract at the Gin transport boundary.
package contract

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	ginmiddleware "github.com/oapi-codegen/gin-middleware"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
)

// FailureKind identifies a transport validation failure without selecting its final HTTP problem response.
type FailureKind string

const (
	// FailureInvalidRequest identifies malformed input or a request that violates the OpenAPI schema.
	FailureInvalidRequest FailureKind = "invalid_request"
	// FailureVersionRequired identifies a missing required If-Match header for future RFC 9457 428 mapping.
	FailureVersionRequired FailureKind = "version_required"
)

// Failure describes an OpenAPI request validation failure for a transport-owned error mapper.
type Failure struct {
	// Kind is the stable classification consumed by the REST error mapper.
	Kind FailureKind
	// Message is the validator detail and must not be used as a stable discriminator.
	Message string
	// Status is the validator's default HTTP status before a transport mapper overrides it.
	Status int
}

// Options customizes validation failure handling without adding business behavior.
type Options struct {
	// ErrorHandler receives a classified failure; nil uses the official middleware's JSON status response.
	ErrorHandler func(*gin.Context, Failure)
}

// Middleware loads the generated embedded specification and returns authoritative request validation middleware.
func Middleware(options Options) (gin.HandlerFunc, error) {
	specification, err := generated.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load embedded OpenAPI specification: %w", err)
	}
	validatorOptions := &ginmiddleware.Options{
		SilenceServersWarning: true,
		ErrorHandler: func(ctx *gin.Context, message string, status int) {
			failure := Failure{Kind: classify(message), Message: message, Status: status}
			if options.ErrorHandler != nil {
				options.ErrorHandler(ctx, failure)
				return
			}
			ctx.AbortWithStatusJSON(status, gin.H{"error": message})
		},
	}
	return ginmiddleware.OapiRequestValidatorWithOptions(specification, validatorOptions), nil
}

// classify converts the official validator detail into the narrow classification needed by Todo 10.
func classify(message string) FailureKind {
	normalized := strings.ToLower(message)
	if strings.Contains(normalized, "if-match") && strings.Contains(normalized, "required") {
		return FailureVersionRequired
	}
	return FailureInvalidRequest
}
