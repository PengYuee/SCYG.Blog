package httpserver

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
)

// transactionWriter buffers an ordinary response until the handler stack returns.
type transactionWriter struct {
	// delegate is the Gin writer that owns the real connection.
	delegate gin.ResponseWriter
	// hijacked is the transferred connection, when any.
	hijacked net.Conn
	// header is isolated from the real response until commit.
	header http.Header
	// baseline contains safe headers installed before recovery.
	baseline http.Header
	// body contains ordinary uncommitted output.
	body bytes.Buffer
	// status is the uncommitted HTTP status.
	status int
	// committed marks streaming or completed output that cannot roll back.
	committed bool
}

// newTransactionWriter starts with headers installed before the recovery boundary.
func newTransactionWriter(delegate gin.ResponseWriter) *transactionWriter {
	baseline := delegate.Header().Clone()
	return &transactionWriter{delegate: delegate, header: baseline.Clone(), baseline: baseline}
}

// Header returns transaction-owned headers that cannot leak before commit.
func (writer *transactionWriter) Header() http.Header { return writer.header }

// WriteHeader records the first status without committing it to the connection.
func (writer *transactionWriter) WriteHeader(status int) {
	if writer.committed {
		writer.delegate.WriteHeader(status)
		return
	}
	if writer.status != 0 {
		return
	}
	writer.status = status
}

// WriteHeaderNow records an implicit successful status without forcing a commit.
func (writer *transactionWriter) WriteHeaderNow() {
	if writer.committed {
		writer.delegate.WriteHeaderNow()
		return
	}
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
}

// Write buffers response bytes until normal middleware completion.
func (writer *transactionWriter) Write(content []byte) (int, error) {
	if writer.committed {
		return writer.delegate.Write(content)
	}
	writer.WriteHeaderNow()
	return writer.body.Write(content)
}

// WriteString buffers response text until normal middleware completion.
func (writer *transactionWriter) WriteString(content string) (int, error) {
	if writer.committed {
		return writer.delegate.WriteString(content)
	}
	writer.WriteHeaderNow()
	return writer.body.WriteString(content)
}

// Status returns the transaction status or the delegate status after commit.
func (writer *transactionWriter) Status() int {
	if writer.committed {
		return writer.delegate.Status()
	}
	if writer.status == 0 {
		return http.StatusOK
	}
	return writer.status
}

// Size returns buffered bytes or the delegate size after commit.
func (writer *transactionWriter) Size() int {
	if writer.committed {
		return writer.delegate.Size()
	}
	return writer.body.Len()
}

// Written reports whether the transaction has a status/body or has committed.
func (writer *transactionWriter) Written() bool {
	return writer.committed || writer.status != 0 || writer.body.Len() != 0
}

// Flush commits the transaction and explicitly enters non-rollback streaming mode.
func (writer *transactionWriter) Flush() {
	if err := writer.commit(); err != nil {
		return
	}
	writer.delegate.Flush()
}

// Hijack transfers an untouched response connection; buffered HTTP output is rejected.
func (writer *transactionWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if writer.status != 0 || writer.body.Len() != 0 {
		return nil, nil, fmt.Errorf("hijack after buffered response write")
	}
	connection, buffer, err := writer.delegate.Hijack()
	if err == nil {
		writer.committed = true
		writer.hijacked = connection
	}
	return connection, buffer, err
}

// CloseNotify delegates legacy disconnect notification semantics.
func (writer *transactionWriter) CloseNotify() <-chan bool { return writer.delegate.CloseNotify() }

// Pusher returns the delegate's optional HTTP/2 server-push implementation.
func (writer *transactionWriter) Pusher() http.Pusher { return writer.delegate.Pusher() }

// commit copies transaction headers, status, and body exactly once.
func (writer *transactionWriter) commit() error {
	if writer.committed {
		return nil
	}
	target := writer.delegate.Header()
	clear(target)
	for key, values := range writer.header {
		target[key] = append([]string(nil), values...)
	}
	writer.delegate.WriteHeader(writer.Status())
	writer.delegate.WriteHeaderNow()
	writer.committed = true
	if writer.body.Len() == 0 {
		return nil
	}
	if _, err := writer.delegate.Write(writer.body.Bytes()); err != nil {
		return fmt.Errorf("commit response: %w", err)
	}
	return nil
}

// reset discards uncommitted handler output while preserving pre-recovery headers.
func (writer *transactionWriter) reset() bool {
	if writer.committed {
		return false
	}
	writer.header = writer.baseline.Clone()
	writer.body.Reset()
	writer.status = 0
	return true
}

// closeHijacked closes a connection already transferred to a panicking handler.
func (writer *transactionWriter) closeHijacked() (bool, error) {
	if writer.hijacked == nil {
		return false, nil
	}
	return true, writer.hijacked.Close()
}
