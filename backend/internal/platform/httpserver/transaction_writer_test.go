package httpserver

import (
	"bufio"
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
	disconnected chan bool
}

func (recorder *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return recorder.connection, bufio.NewReadWriter(bufio.NewReader(recorder.connection), bufio.NewWriter(recorder.connection)), nil
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

func Test_TransactionWriter_Hijack_and_CloseNotify_delegate(t *testing.T) {
	serverConnection, clientConnection := net.Pipe()
	defer func() {
		if closeErr := clientConnection.Close(); closeErr != nil {
			t.Errorf("close client connection: %v", closeErr)
		}
	}()
	recorder := &hijackRecorder{ResponseRecorder: httptest.NewRecorder(), connection: serverConnection, disconnected: make(chan bool, 1)}
	ctx, _ := gin.CreateTestContext(recorder)
	writer := newTransactionWriter(ctx.Writer)

	connection, _, err := writer.Hijack()
	if err != nil {
		t.Fatalf("hijack: %v", err)
	}
	if connection != serverConnection || writer.CloseNotify() != recorder.disconnected {
		t.Fatal("optional writer interfaces were not delegated")
	}
	if closeErr := connection.Close(); closeErr != nil {
		t.Fatalf("close hijacked connection: %v", closeErr)
	}
}
