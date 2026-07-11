package httpserver

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type hijackRecorder struct {
	*httptest.ResponseRecorder
	connection   net.Conn
	buffer       *bufio.ReadWriter
	disconnected chan bool
	hijacked     bool
}

func (recorder *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	recorder.hijacked = true
	return recorder.connection, recorder.buffer, nil
}

func (recorder *hijackRecorder) CloseNotify() <-chan bool { return recorder.disconnected }

func Test_TransactionWriter_buffers_normal_status_headers_and_large_body(t *testing.T) {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	writer := newTransactionWriter(ctx.Writer)
	var _ gin.ResponseWriter = writer
	content := strings.Repeat("response-data", 200_000)

	writer.Header().Set("X-Result", "safe")
	writer.WriteHeader(http.StatusCreated)
	written, err := writer.WriteString(content)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("response committed early: %d/%d", response.Code, response.Body.Len())
	}

	if commitErr := writer.commit(); commitErr != nil {
		t.Fatalf("commit: %v", commitErr)
	}
	if written != len(content) || response.Code != http.StatusCreated || response.Header().Get("X-Result") != "safe" || response.Body.String() != content {
		t.Fatalf("unexpected committed response status=%d size=%d", response.Code, response.Body.Len())
	}
}

func Test_TransactionWriter_Flush_commits_and_preserves_streaming(t *testing.T) {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	writer := newTransactionWriter(ctx.Writer)
	if _, writeErr := writer.WriteString("first"); writeErr != nil {
		t.Fatalf("write first: %v", writeErr)
	}

	writer.Flush()
	if _, writeErr := writer.WriteString("-second"); writeErr != nil {
		t.Fatalf("write second: %v", writeErr)
	}

	if !response.Flushed || response.Body.String() != "first-second" {
		t.Fatalf("flush semantics lost: flushed=%v body=%q", response.Flushed, response.Body.String())
	}
	if writer.reset() {
		t.Fatal("committed streaming response was incorrectly rollbackable")
	}
}

func Test_TransactionWriter_Hijack_returns_error_when_underlying_writer_is_unsupported(t *testing.T) {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	writer := newTransactionWriter(ctx.Writer)

	var connection net.Conn
	var buffer *bufio.ReadWriter
	var hijackErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("Hijack panicked: %v", recovered)
			}
		}()
		connection, buffer, hijackErr = writer.Hijack()
	}()

	if connection != nil || buffer != nil || hijackErr == nil {
		t.Fatalf("connection=%v buffer=%v error=%v", connection, buffer, hijackErr)
	}
	if strings.Contains(strings.ToLower(hijackErr.Error()), "secret") {
		t.Fatalf("unsafe error=%q", hijackErr)
	}
}

func Test_TransactionWriter_Hijack_rejects_buffered_output(t *testing.T) {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	writer := newTransactionWriter(ctx.Writer)
	if _, writeErr := writer.WriteString("buffered"); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	connection, buffer, hijackErr := writer.Hijack()

	if connection != nil || buffer != nil || hijackErr == nil {
		t.Fatalf("connection=%v buffer=%v error=%v", connection, buffer, hijackErr)
	}
}

func Test_TransactionWriter_Hijack_and_CloseNotify_delegate(t *testing.T) {
	var expectedConnection net.Conn = (*net.TCPConn)(nil)
	expectedBuffer := bufio.NewReadWriter(bufio.NewReader(strings.NewReader("")), bufio.NewWriter(io.Discard))
	recorder := &hijackRecorder{ResponseRecorder: httptest.NewRecorder(), connection: expectedConnection, buffer: expectedBuffer, disconnected: make(chan bool, 1)}
	ctx, _ := gin.CreateTestContext(recorder)
	writer := newTransactionWriter(ctx.Writer)

	connection, buffer, err := writer.Hijack()
	if err != nil {
		t.Fatalf("hijack: %v", err)
	}
	if !recorder.hijacked || connection != expectedConnection || buffer != expectedBuffer || writer.CloseNotify() != recorder.disconnected {
		t.Fatal("optional writer interfaces were not delegated")
	}
}
