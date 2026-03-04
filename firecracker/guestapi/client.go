// Package guestapi provides an HTTP client for communicating with a
// vsock-based guest agent running inside a Firecracker VM.
//
// The guest agent (pigeon-init) exposes HTTP/1.1 endpoints for signal
// delivery, status queries, and command execution over vsock. The host
// connects through the Firecracker vsock UDS using the CONNECT protocol.
//
// The guest_api task config block opts in to this feature and configures
// the port. Presence of the block enables vsock-based signal delivery in
// StopTask and SignalTask, preferred over SendCtrlAltDel.
package guestapi

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client communicates with a guest agent over the Firecracker vsock UDS.
type Client struct {
	udsPath string
	port    uint32
	client  *http.Client
}

// New creates a guest API client. udsPath is the path to the Firecracker
// vsock UDS (v.sock inside the jailer chroot). port is the guest-side
// listener port (typically 10000).
func New(udsPath string, port uint32) *Client {
	c := &Client{
		udsPath: udsPath,
		port:    port,
	}
	c.client = &http.Client{
		Transport: &http.Transport{
			DialContext: c.dialVsock,
			// Each vsock CONNECT is a new stream; disable pooling.
			DisableKeepAlives: true,
		},
	}
	return c
}

// dialVsock connects to the guest via Firecracker's vsock UDS protocol:
// connect to the Unix socket, send "CONNECT <port>\n", read "OK <id>\n".
func (c *Client) dialVsock(ctx context.Context, _, _ string) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.udsPath)
	if err != nil {
		return nil, fmt.Errorf("vsock UDS dial: %w", err)
	}

	// Apply context deadline to the handshake.
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			conn.Close()
			return nil, fmt.Errorf("vsock set deadline: %w", err)
		}
	}

	// Firecracker vsock protocol: send "CONNECT <port>\n".
	if _, err := fmt.Fprintf(conn, "CONNECT %d\n", c.port); err != nil {
		conn.Close()
		return nil, fmt.Errorf("vsock CONNECT write: %w", err)
	}

	// Read response line (e.g., "OK 0\n"). Use buffered reader to
	// guarantee we consume exactly one complete line.
	resp, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("vsock CONNECT read: %w", err)
	}
	if !strings.HasPrefix(resp, "OK ") {
		conn.Close()
		return nil, fmt.Errorf("vsock CONNECT rejected: %s", strings.TrimSpace(resp))
	}

	// Clear deadline for subsequent HTTP I/O (context handles cancellation).
	_ = conn.SetDeadline(time.Time{})

	return conn, nil
}

// Signal sends a signal to the guest workload via POST /v1/signals.
// The signal number follows POSIX conventions (e.g., 15 for SIGTERM).
func (c *Client) Signal(ctx context.Context, signal int) error {
	body := fmt.Sprintf(`{"signal":%d}`, signal)
	req, err := http.NewRequestWithContext(ctx, "POST", "http://guest/v1/signals", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST /v1/signals: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST /v1/signals: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}
