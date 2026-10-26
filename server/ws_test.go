package server

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// wsClient is the browser's side of a WebSocket, for the tests.
type wsClient struct {
	t    *testing.T
	conn net.Conn
	r    *bufio.Reader
}

// dialWS opens a WebSocket to url's path on the server, with the headers
// given as name, value pairs.
func dialWS(t *testing.T, srv *httptest.Server, path string, header ...string) (*wsClient, *http.Response) {
	t.Helper()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 16)
	_, _ = rand.Read(key)
	req := "GET " + path + " HTTP/1.1\r\nHost: " + srv.Listener.Addr().String() +
		"\r\nUpgrade: websocket\r\nConnection: keep-alive, Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: " +
		base64.StdEncoding.EncodeToString(key) + "\r\n"
	for i := 0; i+1 < len(header); i += 2 {
		req += header[i] + ": " + header[i+1] + "\r\n"
	}
	if _, err := conn.Write([]byte(req + "\r\n")); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, resp
	}
	if got := resp.Header.Get("Sec-WebSocket-Accept"); got != acceptKey(base64.StdEncoding.EncodeToString(key)) {
		t.Fatalf("accept %q", got)
	}
	c := &wsClient{t: t, conn: conn, r: r}
	t.Cleanup(func() { conn.Close() })
	return c, resp
}

// frame sends a masked frame.
func (c *wsClient) frame(fin bool, op byte, data []byte) {
	c.t.Helper()
	var b bytes.Buffer
	first := op
	if fin {
		first |= 0x80
	}
	b.WriteByte(first)
	switch n := len(data); {
	case n < 126:
		b.WriteByte(0x80 | byte(n))
	case n <= 0xFFFF:
		b.WriteByte(0x80 | 126)
		_ = binary.Write(&b, binary.BigEndian, uint16(n))
	default:
		b.WriteByte(0x80 | 127)
		_ = binary.Write(&b, binary.BigEndian, uint64(n))
	}
	mask := []byte{0x12, 0x34, 0x56, 0x78}
	b.Write(mask)
	for i, x := range data {
		b.WriteByte(x ^ mask[i%4])
	}
	if _, err := c.conn.Write(b.Bytes()); err != nil {
		c.t.Fatal(err)
	}
}

func (c *wsClient) send(text string) { c.frame(true, opText, []byte(text)) }

// read returns the next frame from the server, or an error if none comes
// within the time given.
func (c *wsClient) read(within time.Duration) (op byte, data []byte, err error) {
	_ = c.conn.SetReadDeadline(time.Now().Add(within))
	var h [2]byte
	if _, err := io.ReadFull(c.r, h[:]); err != nil {
		return 0, nil, err
	}
	if h[1]&0x80 != 0 {
		return 0, nil, errors.New("the server masked a frame")
	}
	n := uint64(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		_, _ = io.ReadFull(c.r, b[:])
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		_, _ = io.ReadFull(c.r, b[:])
		n = binary.BigEndian.Uint64(b[:])
	}
	data = make([]byte, n)
	_, err = io.ReadFull(c.r, data)
	return h[0] & 0x0F, data, err
}

// closeCode reads frames until a close frame and returns its code.
func (c *wsClient) closeCode() int {
	c.t.Helper()
	for {
		op, data, err := c.read(5 * time.Second)
		if err != nil {
			c.t.Fatalf("no close frame: %v", err)
		}
		if op == opClose {
			if len(data) < 2 {
				return closeNoStatus
			}
			return int(binary.BigEndian.Uint16(data))
		}
	}
}

func TestAcceptKey(t *testing.T) {
	// The example of RFC 6455, section 1.3.
	if got := acceptKey("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Errorf("acceptKey = %s", got)
	}
}

// echoServer echoes every message; what it reads last is kept in got.
func echoServer(t *testing.T, limit int) (*httptest.Server, chan error) {
	errs := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrade(w, r, limit)
		if err != nil {
			return
		}
		for {
			op, data, err := c.readMessage()
			if err != nil {
				errs <- err
				return
			}
			if err := c.write(op, data); err != nil {
				errs <- err
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv, errs
}

func TestWebSocketMessages(t *testing.T) {
	srv, errs := echoServer(t, 1<<16)
	c, _ := dialWS(t, srv, "/")

	// Short, medium and long messages, text and binary.
	for _, n := range []int{0, 5, 125, 126, 300, 65535} {
		data := bytes.Repeat([]byte("x"), n)
		c.frame(true, opBinary, data)
		op, got, err := c.read(time.Second)
		if err != nil || op != opBinary || !bytes.Equal(got, data) {
			t.Fatalf("%d bytes: op %d, %d bytes, %v", n, op, len(got), err)
		}
	}
	// Fragments, with a ping between them, which is answered at once.
	c.frame(false, opText, []byte("Gro"))
	c.frame(true, opPing, []byte("p"))
	c.frame(false, opContinuation, []byte("ß"))
	c.frame(true, opContinuation, []byte("vater"))
	if op, data, _ := c.read(time.Second); op != opPong || string(data) != "p" {
		t.Errorf("pong: op %d %q", op, data)
	}
	if op, data, _ := c.read(time.Second); op != opText || string(data) != "Großvater" {
		t.Errorf("fragments: op %d %q", op, data)
	}
	// A close is answered with a close.
	c.frame(true, opClose, binary.BigEndian.AppendUint16(nil, closeGoingAway))
	if code := c.closeCode(); code != closeGoingAway {
		t.Errorf("close code %d", code)
	}
	var ce *closeError
	if err := <-errs; !errors.As(err, &ce) || ce.code != closeGoingAway {
		t.Errorf("server: %v", err)
	}
}

// Broken frames close the connection with the code that says why.
func TestWebSocketViolations(t *testing.T) {
	for _, tt := range []struct {
		name string
		send func(c *wsClient)
		code int
	}{
		{"too big", func(c *wsClient) { c.frame(true, opText, bytes.Repeat([]byte("x"), 2000)) }, closeTooBig},
		{"too big in fragments", func(c *wsClient) {
			c.frame(false, opText, bytes.Repeat([]byte("x"), 600))
			c.frame(false, opContinuation, bytes.Repeat([]byte("x"), 600))
		}, closeTooBig},
		{"not UTF-8", func(c *wsClient) { c.frame(true, opText, []byte{0xff, 0xfe}) }, closeInvalidData},
		{"unmasked", func(c *wsClient) { _, _ = c.conn.Write([]byte{0x81, 0x01, 'x'}) }, closeProtocol},
		{"reserved bits", func(c *wsClient) { _, _ = c.conn.Write([]byte{0xC1, 0x80, 0, 0, 0, 0}) }, closeProtocol},
		{"continuation first", func(c *wsClient) { c.frame(true, opContinuation, []byte("x")) }, closeProtocol},
		{"fragmented ping", func(c *wsClient) { c.frame(false, opPing, nil) }, closeProtocol},
		{"unknown opcode", func(c *wsClient) { c.frame(true, 0x3, nil) }, closeProtocol},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv, errs := echoServer(t, 1024)
			c, _ := dialWS(t, srv, "/")
			tt.send(c)
			if code := c.closeCode(); code != tt.code {
				t.Errorf("close code %d, want %d", code, tt.code)
			}
			<-errs
		})
	}
}

func TestWebSocketHandshake(t *testing.T) {
	srv, _ := echoServer(t, 1024)
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("plain GET: %s", resp.Status)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "8")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUpgradeRequired || resp.Header.Get("Sec-WebSocket-Version") != "13" {
		t.Errorf("version 8: %s %v", resp.Status, resp.Header)
	}
	if !strings.Contains(acceptKey("x"), "=") {
		t.Error("acceptKey is not base64")
	}
}
