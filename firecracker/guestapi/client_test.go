package guestapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shoenig/test/must"
)

const testPort uint32 = 10000

// fakeVsockServer simulates Firecracker's vsock UDS protocol:
// accepts connections, reads "CONNECT <port>\n", responds "OK 0\n",
// then serves one HTTP request on the established stream.
type fakeVsockServer struct {
	ln      net.Listener
	handler http.Handler
	port    uint32
}

func newFakeVsockServer(t *testing.T, socketPath string, port uint32, handler http.Handler) *fakeVsockServer {
	t.Helper()
	ln, err := net.Listen("unix", socketPath)
	must.NoError(t, err)
	s := &fakeVsockServer{ln: ln, handler: handler, port: port}
	go s.serve()
	return s
}

func (s *fakeVsockServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *fakeVsockServer) handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Read CONNECT line.
	line, err := reader.ReadString('\n')
	if err != nil {
		return
	}
	line = strings.TrimSpace(line)

	expected := fmt.Sprintf("CONNECT %d", s.port)
	if line != expected {
		fmt.Fprintf(conn, "ERR invalid\n")
		return
	}
	fmt.Fprintf(conn, "OK 0\n")

	// Read HTTP request and serve it.
	req, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Write(conn)
}

func (s *fakeVsockServer) close() {
	s.ln.Close()
}

func TestClient_Signal(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "v.sock")

	var receivedSignal int
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/signals", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Signal int `json:"signal"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad", 400)
			return
		}
		receivedSignal = req.Signal
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprintf(w, `{"ok":true}`)
	})

	srv := newFakeVsockServer(t, sock, testPort, mux)
	defer srv.close()

	client := New(sock, testPort)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Signal(ctx, 15)
	must.NoError(t, err)
	must.Eq(t, 15, receivedSignal)
}

func TestClient_Status(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "v.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprintf(w, `{"ok":true}`)
	})

	srv := newFakeVsockServer(t, sock, testPort, mux)
	defer srv.close()

	client := New(sock, testPort)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ok, err := client.Status(ctx)
	must.NoError(t, err)
	must.True(t, ok)
}

func TestClient_Signal_ConnectFail(t *testing.T) {
	// Non-existent socket should fail.
	client := New("/nonexistent/v.sock", testPort)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := client.Signal(ctx, 15)
	must.Error(t, err)
}

func TestClient_Signal_Rejected(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "v.sock")

	// Server that rejects CONNECT with wrong port.
	ln, err := net.Listen("unix", sock)
	must.NoError(t, err)
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 128)
		conn.Read(buf)
		fmt.Fprintf(conn, "ERR no agent\n")
	}()

	client := New(sock, testPort)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = client.Signal(ctx, 15)
	must.Error(t, err)
	must.StrContains(t, err.Error(), "rejected")
}
