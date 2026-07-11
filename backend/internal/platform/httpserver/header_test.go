package httpserver_test

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
)

func Test_HTTPServer_rejects_header_over_MaxHeaderBytes_on_real_connection(t *testing.T) {
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.GET("/ok", func(ctx *gin.Context) { ctx.Status(204) })
		return nil
	})
	server.HTTPServer().Addr = "127.0.0.1:0"
	listener, serveErrors, err := server.Start()
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := server.HTTPServer().Close(); closeErr != nil {
			t.Errorf("close server: %v", closeErr)
		}
		<-serveErrors
	})
	connection, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			t.Errorf("close connection: %v", closeErr)
		}
	}()
	request := fmt.Sprintf("GET /ok HTTP/1.1\r\nHost: test\r\nX-Large: %s\r\n\r\n", strings.Repeat("x", httpserver.MaxHeaderBytes+8192))
	if _, writeErr := connection.Write([]byte(request)); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
	response, err := bufio.NewReader(connection).ReadString('\n')
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	if !strings.Contains(response, "431") {
		t.Fatalf("status line=%q", response)
	}
}
