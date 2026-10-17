package rest

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	pb "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
)

func (h agentHandler) publicRate(ctx *gin.Context, bucket string, tolerateUnavailable bool) bool {
	if bucket != "session" && agentScope(ctx) != pb.ContentScope_CONTENT_SCOPE_PUBLIC {
		return true
	}
	if h.Public == nil || h.Public.Control == nil {
		WriteAgentProblem(ctx, http.StatusServiceUnavailable, "公共搜索暂不可用", "public_search_unavailable")
		return false
	}
	ip, err := netip.ParseAddr(ctx.ClientIP())
	if err != nil {
		WriteAgentProblem(ctx, http.StatusServiceUnavailable, "无法建立公共搜索请求", "public_search_unavailable")
		return false
	}
	allowed, retry, err := h.Public.Control.Allow(ctx.Request.Context(), bucket, ip.Unmap().String())
	if err != nil {
		if tolerateUnavailable {
			return true
		}
		WriteAgentProblem(ctx, http.StatusServiceUnavailable, "公共搜索暂不可用", "public_search_unavailable")
		return false
	}
	if !allowed {
		seconds := int64(math.Ceil(retry.Seconds()))
		if seconds < 0 {
			seconds = 0
		}
		ctx.Header("Retry-After", strconv.FormatInt(seconds, 10))
		WriteAgentProblem(ctx, http.StatusTooManyRequests, "请求过于频繁，请稍后重试", "rate_limited")
		return false
	}
	return true
}

func (h agentHandler) publicSession(ctx *gin.Context) {
	raw, ok := agentBody(ctx)
	if !ok {
		return
	}
	if len(raw) != 0 {
		agentBadRequest(ctx)
		return
	}
	if !h.publicRate(ctx, "session", false) {
		return
	}
	token, expires, err := h.Public.Tokens.Issue()
	if err != nil {
		WriteAgentProblem(ctx, 500, "无法签发公共搜索凭证", "")
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(http.StatusOK, gin.H{"accessToken": token, "expiresAt": expires, "queryMaxChars": h.Public.Config.QueryMaxChars(), "maxResults": h.Public.Config.MaxResults()})
}

func (h agentHandler) publicSearch(ctx *gin.Context) {
	if agentScope(ctx) != pb.ContentScope_CONTENT_SCOPE_PUBLIC {
		abortAuthentication(ctx, "需要公共搜索凭证")
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
	var input struct {
		Query      string          `json:"query"`
		MaxResults json.RawMessage `json:"max_results"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || strings.TrimSpace(input.Query) == "" || utf8.RuneCountInString(input.Query) > h.Public.Config.QueryMaxChars() {
		agentBadRequest(ctx)
		return
	}
	maxResults := h.Public.Config.MaxResults()
	if input.MaxResults != nil {
		maxResults = 0
		if json.Unmarshal(input.MaxResults, &maxResults) != nil {
			agentBadRequest(ctx)
			return
		}
	}
	if maxResults < 1 || maxResults > h.Public.Config.MaxResults() {
		agentBadRequest(ctx)
		return
	}
	if !h.publicRate(ctx, "search", false) {
		return
	}
	// Normalize the configured public default explicitly; Agent never receives an unbounded default.
	payload, err := json.Marshal(struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}{input.Query, maxResults})
	if err != nil {
		WriteAgentProblem(ctx, 500, "公共搜索请求失败", "")
		return
	}
	run, err := h.Client.CreateRun(ctx.Request.Context(), &pb.CreateRunRequest{UserId: agentUser(ctx), ContentScope: pb.ContentScope_CONTENT_SCOPE_PUBLIC, IdempotencyKey: key, Capability: pb.AgentCapability_AGENT_CAPABILITY_SEARCH, JsonPayload: payload})
	h.respond(ctx, http.StatusAccepted, run, err)
}
