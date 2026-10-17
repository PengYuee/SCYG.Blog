package rest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	pb "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/authentication"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/publicsearch"
)

const agentBodyLimit = 1 << 20

var (
	runIDPattern        = regexp.MustCompile(`^[A-Za-z0-9._~-]{1,128}$`)
	operationKeyPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89aAbB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
)

// AgentClient is the control capability consumed by REST, without connection ownership or call options.
type AgentClient interface {
	CreateRun(context.Context, *pb.CreateRunRequest) (*pb.Run, error)
	GetRun(context.Context, *pb.GetRunRequest) (*pb.Run, error)
	ResumeRun(context.Context, *pb.ResumeRunRequest) (*pb.Run, error)
	CancelRun(context.Context, *pb.CancelRunRequest) (*pb.Run, error)
	StreamRunEvents(context.Context, *pb.StreamRunEventsRequest) (grpc.ServerStreamingClient[pb.RunEventFrame], error)
}

// AgentOptions supplies the control client and independent subscription budgets.
type AgentOptions struct {
	Client       AgentClient
	ReadyTimeout time.Duration
	IdleTimeout  time.Duration
	Public       *PublicSearchOptions
}

// PublicSearchOptions supplies independent credentials, limits and shared Redis controls.
type PublicSearchOptions struct {
	Tokens  *authentication.PublicSearchTokens
	Control *publicsearch.Control
	Config  config.PublicSearch
}

type agentHandler struct{ AgentOptions }

func validRunID(id string) bool {
	return id != "." && id != ".." && runIDPattern.MatchString(id)
}

func agentUser(ctx *gin.Context) string {
	if subject, ok := authentication.AnonymousSubject(ctx.Request.Context()); ok {
		return subject
	}
	principal, _ := identityauth.PrincipalFromContext(ctx.Request.Context())
	return string(principal.UserID)
}

func agentScope(ctx *gin.Context) pb.ContentScope {
	if _, ok := authentication.AnonymousSubject(ctx.Request.Context()); ok {
		return pb.ContentScope_CONTENT_SCOPE_PUBLIC
	}
	return pb.ContentScope_CONTENT_SCOPE_AUTHOR
}

func agentAuthorized(ctx *gin.Context, run *pb.Run) bool {
	// Agent authorizes the authenticated owner against persistence; Blog independently checks frozen scope.
	if run == nil || run.ContentScope != agentScope(ctx) || (run.ContentScope == pb.ContentScope_CONTENT_SCOPE_PUBLIC && run.Capability != pb.AgentCapability_AGENT_CAPABILITY_SEARCH) {
		WriteAgentProblem(ctx, http.StatusNotFound, "Run 不存在", "RUN_NOT_FOUND")
		return false
	}
	return true
}

func agentBadRequest(ctx *gin.Context) {
	WriteAgentProblem(ctx, http.StatusBadRequest, "请求参数无效", "")
}

func agentKey(ctx *gin.Context) string {
	values := ctx.Request.Header.Values("Idempotency-Key")
	if len(values) != 1 || !operationKeyPattern.MatchString(values[0]) {
		agentBadRequest(ctx)
		return ""
	}
	return values[0]
}

func agentID(ctx *gin.Context) string {
	id := ctx.Param("runId")
	if !validRunID(id) {
		agentBadRequest(ctx)
		return ""
	}
	return id
}

func agentBody(ctx *gin.Context) ([]byte, bool) {
	raw, err := io.ReadAll(io.LimitReader(ctx.Request.Body, agentBodyLimit+1))
	if ctx.IsAborted() {
		return nil, false
	}
	var oversized *http.MaxBytesError
	if len(raw) > agentBodyLimit || errors.As(err, &oversized) {
		AgentRequestTooLarge(ctx)
		return nil, false
	}
	if err != nil {
		agentBadRequest(ctx)
		return nil, false
	}
	return raw, true
}

func agentJSON(ctx *gin.Context) []byte {
	media, _, err := mime.ParseMediaType(ctx.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		WriteAgentProblem(ctx, http.StatusUnsupportedMediaType, "需要 application/json 请求体", "")
		return nil
	}
	raw, ok := agentBody(ctx)
	if !ok {
		return nil
	}
	if !utf8.Valid(raw) || !json.Valid(raw) {
		agentBadRequest(ctx)
		return nil
	}
	return raw
}

func (h agentHandler) register(engine *gin.Engine) {
	for path, capability := range map[string]pb.AgentCapability{
		"search": pb.AgentCapability_AGENT_CAPABILITY_SEARCH,
		"write":  pb.AgentCapability_AGENT_CAPABILITY_WRITE,
		"polish": pb.AgentCapability_AGENT_CAPABILITY_POLISH,
		"chat":   pb.AgentCapability_AGENT_CAPABILITY_CHAT,
	} {
		engine.POST("/api/v1/ai/"+path, func(ctx *gin.Context) {
			key := agentKey(ctx)
			if key == "" {
				return
			}
			raw := agentJSON(ctx)
			if raw == nil {
				return
			}
			run, err := h.Client.CreateRun(ctx.Request.Context(), &pb.CreateRunRequest{
				UserId: agentUser(ctx), ContentScope: pb.ContentScope_CONTENT_SCOPE_AUTHOR, IdempotencyKey: key, Capability: capability, JsonPayload: raw,
			})
			h.respond(ctx, http.StatusAccepted, run, err)
		})
	}
	engine.GET("/api/v1/runs/:runId", h.get)
	engine.GET("/api/v1/runs/:runId/events", h.events)
	engine.POST("/api/v1/runs/:runId/resume", h.resume)
	engine.POST("/api/v1/runs/:runId/cancel", h.cancel)
	if h.Public != nil {
		engine.POST("/api/v1/public/ai/session", h.publicSession)
		engine.POST("/api/v1/public/ai/search", h.publicSearch)
	}
}

func (h agentHandler) get(ctx *gin.Context) {
	id := agentID(ctx)
	if id == "" {
		return
	}
	if !h.publicRate(ctx, "read", true) {
		return
	}
	run, err := h.Client.GetRun(ctx.Request.Context(), &pb.GetRunRequest{UserId: agentUser(ctx), ContentScope: agentScope(ctx), RunId: id})
	h.respond(ctx, http.StatusOK, run, err)
}

func (h agentHandler) resume(ctx *gin.Context) {
	id := agentID(ctx)
	if id == "" {
		return
	}
	key := agentKey(ctx)
	if key == "" {
		return
	}
	raw := agentJSON(ctx)
	if raw == nil {
		return
	}
	var body struct {
		InteractionID string          `json:"interactionId"`
		Decision      string          `json:"decision"`
		Payload       json.RawMessage `json:"payload"`
	}
	// RawMessage preserves omitted payload as nil and explicit null as JSON bytes.
	if json.Unmarshal(raw, &body) != nil || len(raw) == 0 {
		agentBadRequest(ctx)
		return
	}
	if len(body.InteractionID) < 1 || len(body.InteractionID) > 128 || body.Decision == "" {
		agentBadRequest(ctx)
		return
	}
	run, err := h.Client.ResumeRun(ctx.Request.Context(), &pb.ResumeRunRequest{
		UserId: agentUser(ctx), ContentScope: pb.ContentScope_CONTENT_SCOPE_AUTHOR, RunId: id, IdempotencyKey: key,
		InteractionId: body.InteractionID, Decision: body.Decision, PayloadJson: body.Payload,
	})
	h.respond(ctx, http.StatusOK, run, err)
}

func (h agentHandler) cancel(ctx *gin.Context) {
	id := agentID(ctx)
	if id == "" {
		return
	}
	key := agentKey(ctx)
	if key == "" {
		return
	}
	raw, ok := agentBody(ctx)
	if !ok {
		return
	}
	if len(raw) != 0 {
		agentBadRequest(ctx)
		return
	}
	if !h.publicRate(ctx, "read", true) {
		return
	}
	run, err := h.Client.CancelRun(ctx.Request.Context(), &pb.CancelRunRequest{UserId: agentUser(ctx), ContentScope: agentScope(ctx), RunId: id, IdempotencyKey: key})
	h.respond(ctx, http.StatusOK, run, err)
}

func (h agentHandler) respond(ctx *gin.Context, httpStatus int, run *pb.Run, err error) {
	if err != nil {
		agentFailure(ctx, err)
		return
	}
	if !agentAuthorized(ctx, run) {
		return
	}
	if run == nil || !validRunID(run.RunId) {
		WriteAgentProblem(ctx, 500, "Agent 请求失败", "")
		return
	}
	value := mapAgentRun(run)
	if value == nil {
		WriteAgentProblem(ctx, 500, "Agent 请求失败", "")
		return
	}
	ctx.JSON(httpStatus, value)
}

func mapAgentRun(run *pb.Run) gin.H {
	var runStatus, capability string
	switch run.Status {
	case pb.RunStatus_RUN_STATUS_QUEUED:
		runStatus = "queued"
	case pb.RunStatus_RUN_STATUS_RUNNING:
		runStatus = "running"
	case pb.RunStatus_RUN_STATUS_WAITING_FOR_APPROVAL:
		runStatus = "waitingForApproval"
	case pb.RunStatus_RUN_STATUS_SUCCEEDED:
		runStatus = "succeeded"
	case pb.RunStatus_RUN_STATUS_FAILED:
		runStatus = "failed"
	case pb.RunStatus_RUN_STATUS_CANCELLED:
		runStatus = "cancelled"
	case pb.RunStatus_RUN_STATUS_UNSPECIFIED:
		return nil
	default:
		return nil
	}
	switch run.Capability {
	case pb.AgentCapability_AGENT_CAPABILITY_SEARCH:
		capability = "search"
	case pb.AgentCapability_AGENT_CAPABILITY_WRITE:
		capability = "write"
	case pb.AgentCapability_AGENT_CAPABILITY_POLISH:
		capability = "polish"
	case pb.AgentCapability_AGENT_CAPABILITY_CHAT:
		capability = "chat"
	case pb.AgentCapability_AGENT_CAPABILITY_UNSPECIFIED:
		return nil
	default:
		return nil
	}
	value := gin.H{
		"runId": run.RunId, "status": runStatus, "capability": capability,
		"recipeId": run.RecipeId, "recipeVersion": run.RecipeVersion,
		"createdAt": run.CreatedAt, "updatedAt": run.UpdatedAt,
		"failure": nil, "result": nil, "pendingInteraction": nil,
		"streamUrl": "/api/v1/runs/" + run.RunId + "/events",
	}
	if run.Failure != nil {
		value["failure"] = gin.H{"code": run.Failure.Code, "message": run.Failure.Message}
	}
	if run.ResultJson != nil {
		value["result"] = json.RawMessage(run.ResultJson)
	}
	if pending := run.PendingInteraction; pending != nil {
		var interactionType string
		switch pending.Type {
		case pb.InteractionType_INTERACTION_TYPE_CONFIRMATION:
			interactionType = "confirmation"
		case pb.InteractionType_INTERACTION_TYPE_SELECTION:
			interactionType = "selection"
		case pb.InteractionType_INTERACTION_TYPE_TEXT_INPUT:
			interactionType = "textInput"
		case pb.InteractionType_INTERACTION_TYPE_UNSPECIFIED:
			return nil
		default:
			return nil
		}
		value["pendingInteraction"] = gin.H{"interactionId": pending.InteractionId, "type": interactionType, "payload": json.RawMessage(pending.PayloadJson)}
	}
	return value
}
