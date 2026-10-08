package rest

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
)

func (h agentHandler) events(ctx *gin.Context) {
	controller := http.NewResponseController(ctx.Writer)
	if err := controller.SetWriteDeadline(time.Now().Add(2*h.ReadyTimeout + h.IdleTimeout)); err != nil {
		WriteAgentProblem(ctx, 500, "无法建立事件订阅", "")
		return
	}
	id := agentID(ctx)
	if id == "" {
		return
	}
	var cursor *string
	if values, exists := ctx.Request.Header["Last-Event-Id"]; exists {
		value := ""
		if len(values) == 1 {
			value = values[0]
		}
		if len(value) < 1 || len(value) > 256 {
			agentBadRequest(ctx)
			return
		}
		for i := range len(value) {
			if value[i] > 127 {
				agentBadRequest(ctx)
				return
			}
		}
		cursor = &value
	}
	user := agentUser(ctx)
	if _, err := h.Client.GetRun(ctx.Request.Context(), &pb.GetRunRequest{UserId: user, RunId: id}); err != nil {
		agentFailure(ctx, err)
		return
	}
	streamContext, cancel := context.WithCancel(ctx.Request.Context())
	defer cancel()
	timer := time.AfterFunc(h.ReadyTimeout, cancel)
	stream, err := h.Client.StreamRunEvents(streamContext, &pb.StreamRunEventsRequest{UserId: user, RunId: id, AfterEventId: cursor})
	if err != nil {
		if !timer.Stop() && ctx.Request.Context().Err() == nil {
			err = status.Error(codes.DeadlineExceeded, "subscription timeout")
		}
		agentFailure(ctx, err)
		return
	}
	headers, err := stream.Header()
	if err == nil {
		ready := headers.Get("scyg-subscription-ready")
		if len(ready) != 1 || ready[0] != "1" {
			// Header may report nil headers and nil error on a trailers-only failure.
			_, terminal := stream.Recv()
			if terminal != nil {
				err = terminal
			} else {
				err = status.Error(codes.Internal, "missing subscription readiness")
			}
		}
	}
	stopped := timer.Stop()
	if !stopped && ctx.Request.Context().Err() == nil {
		err = status.Error(codes.DeadlineExceeded, "subscription timeout")
	}
	if err != nil {
		agentFailure(ctx, err)
		return
	}
	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("X-Accel-Buffering", "no")
	ctx.Status(http.StatusOK)
	if err := controller.SetWriteDeadline(time.Now().Add(h.IdleTimeout)); err != nil {
		return
	}
	if err := controller.Flush(); err != nil {
		return
	}
	type received struct {
		frame *pb.RunEventFrame
		err   error
		at    time.Time
	}
	frames := make(chan received)
	go func() {
		for {
			frame, err := stream.Recv()
			at := time.Now()
			select {
			case frames <- received{frame, err, at}:
			case <-streamContext.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	idle := time.NewTimer(h.IdleTimeout)
	defer idle.Stop()
	for {
		select {
		case <-streamContext.Done():
			return
		case <-idle.C:
			return
		case item := <-frames:
			if item.err != nil {
				return
			}
			remaining := h.IdleTimeout - time.Since(item.at)
			if remaining <= 0 {
				return
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(remaining)
			if controller.SetWriteDeadline(time.Now().Add(h.IdleTimeout)) != nil {
				return
			}
			if _, err := ctx.Writer.Write(item.frame.Frame); err != nil {
				return
			}
			if controller.Flush() != nil {
				return
			}
		}
	}
}
