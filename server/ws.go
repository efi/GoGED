package server

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// The WebSocket protocol (RFC 6455) as far as the server needs it: the
// handshake, and text, binary, ping, pong and close messages, without
// extensions. Messages from the browser are masked, the server's are not.

// wsGUID is the protocol's fixed key suffix for the handshake.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Frame opcodes.
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// Close codes; 4000 and above are the application's own.
const (
	closeNormal      = 1000
	closeGoingAway   = 1001
	closeProtocol    = 1002
	closeInvalidData = 1007
	closePolicy      = 1008
	closeTooBig      = 1009
)

// writeTimeout is how long a frame may take to send before the browser is
// given up.
const writeTimeout = 30 * time.Second

// acceptKey answers the browser's Sec-WebSocket-Key.
func acceptKey(key string) string {
	sum := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// headerHas reports whether a header lists token, ignoring case.
func headerHas(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, t := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), token) {
				return true
			}
		}
	}
	return false
}

// wsConn is a WebSocket connection. One goroutine reads; writes may come
// from several.
type wsConn struct {
	conn  net.Conn
	r     *bufio.Reader
	limit int // the largest message read, in bytes
	// readTimeout, if set, is how long a frame may take to arrive.
	readTimeout time.Duration

	mu      sync.Mutex // guards writes and closed
	closed  bool       // a close frame was sent
	scratch []byte
}

// upgrade answers a WebSocket handshake and takes the connection over
// from the HTTP server. If the request is not a valid handshake it answers
// with an HTTP error.
func upgrade(w http.ResponseWriter, r *http.Request, limit int) (*wsConn, error) {
	if r.Method != http.MethodGet || !headerHas(r.Header, "Connection", "upgrade") || !headerHas(r.Header, "Upgrade", "websocket") {
		http.Error(w, "a WebSocket handshake is expected here", http.StatusBadRequest)
		return nil, errors.New("not a WebSocket handshake")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, "WebSocket version 13 is expected", http.StatusUpgradeRequired)
		return nil, errors.New("unsupported WebSocket version")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if k, err := base64.StdEncoding.DecodeString(key); err != nil || len(k) != 16 {
		http.Error(w, "invalid Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("invalid Sec-WebSocket-Key")
	}
	conn, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		http.Error(w, "the connection cannot be taken over", http.StatusInternalServerError)
		return nil, err
	}
	reply := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + acceptKey(key) + "\r\n\r\n"
	_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if _, err := conn.Write([]byte(reply)); err != nil {
		conn.Close()
		return nil, err
	}
	return &wsConn{conn: conn, r: rw.Reader, limit: limit}, nil
}

// closeError is a close the connection ended with.
type closeError struct {
	code   int
	reason string
}

func (e *closeError) Error() string { return fmt.Sprintf("WebSocket closed: %d %s", e.code, e.reason) }

// readMessage returns the next text or binary message. It answers pings
// and joins fragments on the way. When the browser closes the connection,
// or breaks the protocol, it answers with a close frame and returns a
// *closeError.
func (c *wsConn) readMessage() (op byte, data []byte, err error) {
	for {
		fin, fop, payload, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}
		switch fop {
		case opPing:
			if err := c.write(opPong, payload); err != nil {
				return 0, nil, err
			}
			continue
		case opPong:
			continue
		case opClose:
			code, reason := closeNoStatus, ""
			if len(payload) >= 2 {
				code, reason = int(binary.BigEndian.Uint16(payload)), string(payload[2:])
			}
			reply := code
			if code == closeNoStatus {
				reply = closeNormal
			}
			c.Close(reply, "")
			return 0, nil, &closeError{code, reason}
		case opText, opBinary:
			if op != 0 {
				return 0, nil, c.fail(closeProtocol, "a new message began within a fragmented one")
			}
			op, data = fop, payload
		case opContinuation:
			if op == 0 {
				return 0, nil, c.fail(closeProtocol, "a continuation without a message")
			}
			data = append(data, payload...)
		default:
			return 0, nil, c.fail(closeProtocol, fmt.Sprintf("unknown opcode %d", fop))
		}
		if len(data) > c.limit {
			return 0, nil, c.fail(closeTooBig, "message too big")
		}
		if fin {
			if op == opText && !utf8.Valid(data) {
				return 0, nil, c.fail(closeInvalidData, "text is not UTF-8")
			}
			return op, data, nil
		}
	}
}

// closeNoStatus stands for a close frame without a code.
const closeNoStatus = 1005

// readFrame reads one frame and unmasks its payload.
func (c *wsConn) readFrame() (fin bool, op byte, payload []byte, err error) {
	if c.readTimeout > 0 {
		_ = c.conn.SetReadDeadline(time.Now().Add(c.readTimeout))
	}
	var h [2]byte
	if _, err := io.ReadFull(c.r, h[:]); err != nil {
		return false, 0, nil, err
	}
	fin, op = h[0]&0x80 != 0, h[0]&0x0F
	if h[0]&0x70 != 0 {
		return false, 0, nil, c.fail(closeProtocol, "reserved bits are set")
	}
	if h[1]&0x80 == 0 {
		return false, 0, nil, c.fail(closeProtocol, "frames from the browser must be masked")
	}
	n := uint64(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(c.r, b[:]); err != nil {
			return false, 0, nil, err
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(c.r, b[:]); err != nil {
			return false, 0, nil, err
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if op >= opClose && (n > 125 || !fin) {
		return false, 0, nil, c.fail(closeProtocol, "invalid control frame")
	}
	if n > uint64(c.limit) {
		return false, 0, nil, c.fail(closeTooBig, "message too big")
	}
	var mask [4]byte
	if _, err := io.ReadFull(c.r, mask[:]); err != nil {
		return false, 0, nil, err
	}
	payload = make([]byte, n)
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return false, 0, nil, err
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return fin, op, payload, nil
}

// fail closes the connection for a broken protocol and returns the error.
func (c *wsConn) fail(code int, reason string) error {
	c.Close(code, reason)
	return &closeError{code, reason}
}

// errClosed is returned for writes after the close frame.
var errClosed = errors.New("WebSocket connection closed")

// write sends a message in one frame.
func (c *wsConn) write(op byte, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errClosed
	}
	return c.frame(op, data)
}

// frame writes one frame; c.mu is held.
func (c *wsConn) frame(op byte, data []byte) error {
	b := append(c.scratch[:0], 0x80|op)
	switch n := len(data); {
	case n < 126:
		b = append(b, byte(n))
	case n <= 0xFFFF:
		b = append(b, 126)
		b = binary.BigEndian.AppendUint16(b, uint16(n))
	default:
		b = append(b, 127)
		b = binary.BigEndian.AppendUint64(b, uint64(n))
	}
	c.scratch = b
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	bufs := net.Buffers{b, data}
	_, err := bufs.WriteTo(c.conn)
	return err
}

func (c *wsConn) writeText(data []byte) error   { return c.write(opText, data) }
func (c *wsConn) writeBinary(data []byte) error { return c.write(opBinary, data) }
func (c *wsConn) ping() error                   { return c.write(opPing, nil) }

// Close sends a close frame with code and reason, unless one was sent,
// and closes the connection.
func (c *wsConn) Close(code int, reason string) {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		if len(reason) > 123 {
			reason = reason[:123]
		}
		_ = c.frame(opClose, append(binary.BigEndian.AppendUint16(nil, uint16(code)), reason...))
	}
	c.mu.Unlock()
	c.conn.Close()
}
