/*
 * Copyright (c) NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package systemd

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

type Server struct {
	path string
	ln   net.Listener

	mu    sync.Mutex
	conns map[net.Conn]struct{}

	wg   sync.WaitGroup
	once sync.Once
}

func Start(path string) (*Server, error) {
	if err := os.RemoveAll(path); err != nil {
		return nil, err
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	s := &Server{
		path:  path,
		ln:    ln,
		conns: make(map[net.Conn]struct{}),
	}

	s.wg.Add(1)
	go s.serve()

	return s, nil
}

func (s *Server) Address() string {
	return "unix:path=" + s.path
}

func (s *Server) Close() error {
	var closeErr error

	s.once.Do(func() {
		// Stop accepting new connections.
		closeErr = s.ln.Close()

		// Close existing connections. This also unblocks any goroutines
		// currently waiting in readMessage().
		s.mu.Lock()
		for conn := range s.conns {
			_ = conn.Close()
		}
		s.mu.Unlock()

		s.wg.Wait()

		_ = os.Remove(s.path)
	})

	return closeErr
}

func (s *Server) serve() {
	defer s.wg.Done()

	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}

		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		s.wg.Add(1)

		go func(conn net.Conn) {
			defer s.wg.Done()
			defer conn.Close()

			defer func() {
				s.mu.Lock()
				delete(s.conns, conn)
				s.mu.Unlock()
			}()

			_ = handleConnection(conn)
		}(conn)
	}
}

// -----------------------------------------------------------------------------
// D-Bus authentication
// -----------------------------------------------------------------------------

func handleConnection(conn net.Conn) error {
	r := bufio.NewReader(conn)

	// D-Bus authentication starts with a NUL byte.
	b, err := r.ReadByte()
	if err != nil {
		return err
	}
	if b != 0 {
		return fmt.Errorf("expected initial NUL, got %#x", b)
	}

	// godbus first asks which authentication mechanisms are supported.
	line, err := readAuthLine(r)
	if err != nil {
		return err
	}
	if line != "AUTH" {
		return fmt.Errorf("expected AUTH, got %q", line)
	}

	if err := writeAuthLine(conn, "REJECTED EXTERNAL"); err != nil {
		return err
	}

	// godbus selects EXTERNAL. The optional argument is the
	// hex-encoded authorization identity.
	line, err = readAuthLine(r)
	if err != nil {
		return err
	}

	if line != "AUTH EXTERNAL" &&
		!strings.HasPrefix(line, "AUTH EXTERNAL ") {
		return fmt.Errorf("expected AUTH EXTERNAL, got %q", line)
	}

	// Deliberately don't validate the identity.
	if err := writeAuthLine(
		conn,
		"OK 0123456789abcdef0123456789abcdef",
	); err != nil {
		return err
	}

	// godbus may negotiate Unix FD passing before BEGIN.
	for {
		line, err = readAuthLine(r)
		if err != nil {
			return err
		}

		switch line {
		case "NEGOTIATE_UNIX_FD":
			// This fake bus doesn't support FD passing.
			if err := writeAuthLine(conn, "ERROR"); err != nil {
				return err
			}

		case "BEGIN":
			// Everything after BEGIN is binary D-Bus traffic.
			return handleDBus(conn, r)

		default:
			return fmt.Errorf(
				"unexpected authentication command %q",
				line,
			)
		}
	}
}

func readAuthLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}

	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")

	return line, nil
}

func writeAuthLine(w io.Writer, line string) error {
	_, err := io.WriteString(w, line+"\r\n")
	return err
}

// -----------------------------------------------------------------------------
// D-Bus message handling
// -----------------------------------------------------------------------------

const (
	messageTypeMethodCall   byte = 1
	messageTypeMethodReturn byte = 2
	messageTypeError        byte = 3
)

type message struct {
	order   binary.ByteOrder
	msgType byte
	serial  uint32

	path        string
	iface       string
	member      string
	destination string
	signature   string

	body []byte
}

func handleDBus(conn net.Conn, r io.Reader) error {
	// The first D-Bus message from a bus client must be Hello().
	msg, err := readMessage(r)
	if err != nil {
		return err
	}

	if msg.msgType != messageTypeMethodCall {
		return fmt.Errorf(
			"expected METHOD_CALL, got message type %d",
			msg.msgType,
		)
	}

	if msg.path != "/org/freedesktop/DBus" {
		return fmt.Errorf(
			"expected /org/freedesktop/DBus, got %q",
			msg.path,
		)
	}

	if msg.iface != "org.freedesktop.DBus" {
		return fmt.Errorf(
			"expected org.freedesktop.DBus interface, got %q",
			msg.iface,
		)
	}

	if msg.member != "Hello" {
		return fmt.Errorf(
			"expected Hello, got %q",
			msg.member,
		)
	}

	if msg.destination != "org.freedesktop.DBus" {
		return fmt.Errorf(
			"expected org.freedesktop.DBus destination, got %q",
			msg.destination,
		)
	}

	if err := writeHelloReply(conn, msg.serial); err != nil {
		return err
	}

	// IMPORTANT:
	//
	// Keep the connection open. godbus has a reader goroutine monitoring
	// the socket. Closing it immediately after Hello would cause
	// go-systemd's Conn.Connected() to become false.
	//
	// Any further METHOD_CALL receives UnknownMethod. This makes accidental
	// systemd operations fail immediately instead of hanging forever.
	for {
		msg, err := readMessage(r)
		if err != nil {
			return err
		}

		if msg.msgType != messageTypeMethodCall {
			// Signals, returns, etc. don't need a response.
			continue
		}

		if err := writeUnknownMethodReply(
			conn,
			msg.serial,
			msg.iface,
			msg.member,
		); err != nil {
			return err
		}
	}
}

// -----------------------------------------------------------------------------
// D-Bus message decoder
// -----------------------------------------------------------------------------

func readMessage(r io.Reader) (*message, error) {
	var fixed [16]byte

	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		return nil, err
	}

	var order binary.ByteOrder

	switch fixed[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		return nil, fmt.Errorf(
			"invalid D-Bus byte order %#x",
			fixed[0],
		)
	}

	if fixed[3] != 1 {
		return nil, fmt.Errorf(
			"unsupported D-Bus protocol version %d",
			fixed[3],
		)
	}

	bodyLen := order.Uint32(fixed[4:8])
	serial := order.Uint32(fixed[8:12])
	fieldsLen := order.Uint32(fixed[12:16])

	fields := make([]byte, fieldsLen)

	if _, err := io.ReadFull(r, fields); err != nil {
		return nil, err
	}

	// The body begins on an 8-byte boundary.
	headerSize := 16 + int(fieldsLen)
	padding := align(headerSize, 8) - headerSize

	if padding > 0 {
		pad := make([]byte, padding)

		if _, err := io.ReadFull(r, pad); err != nil {
			return nil, err
		}
	}

	body := make([]byte, bodyLen)

	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}

	m := &message{
		order:   order,
		msgType: fixed[1],
		serial:  serial,
		body:    body,
	}

	if err := parseHeaderFields(m, fields); err != nil {
		return nil, err
	}

	return m, nil
}

// D-Bus header fields have type a(yv).
//
// Fields relevant to this fake:
//
//	1 = PATH
//	2 = INTERFACE
//	3 = MEMBER
//	6 = DESTINATION
//	8 = SIGNATURE
func parseHeaderFields(m *message, data []byte) error {
	pos := 0

	for pos < len(data) {
		// Every (yv) struct is aligned to 8 bytes.
		pos = align(pos, 8)

		if pos >= len(data) {
			break
		}

		code := data[pos]
		pos++

		// Variant signature:
		//
		// BYTE length
		// BYTE[length] signature
		// NUL
		if pos >= len(data) {
			return io.ErrUnexpectedEOF
		}

		sigLen := int(data[pos])
		pos++

		if pos+sigLen+1 > len(data) {
			return io.ErrUnexpectedEOF
		}

		sig := string(data[pos : pos+sigLen])
		pos += sigLen

		if data[pos] != 0 {
			return fmt.Errorf(
				"variant signature is not NUL terminated",
			)
		}
		pos++

		switch sig {
		case "s", "o":
			pos = align(pos, 4)

			value, next, err := readString(
				m.order,
				data,
				pos,
			)
			if err != nil {
				return err
			}

			pos = next

			switch code {
			case 1:
				m.path = value
			case 2:
				m.iface = value
			case 3:
				m.member = value
			case 6:
				m.destination = value
			}

		case "g":
			value, next, err := readSignature(data, pos)
			if err != nil {
				return err
			}

			pos = next

			if code == 8 {
				m.signature = value
			}

		case "u":
			pos = align(pos, 4)

			if pos+4 > len(data) {
				return io.ErrUnexpectedEOF
			}

			pos += 4

		default:
			return fmt.Errorf(
				"unsupported header variant signature %q",
				sig,
			)
		}
	}

	return nil
}

func readString(
	order binary.ByteOrder,
	data []byte,
	pos int,
) (string, int, error) {
	if pos+4 > len(data) {
		return "", 0, io.ErrUnexpectedEOF
	}

	n := int(order.Uint32(data[pos : pos+4]))
	pos += 4

	if pos+n+1 > len(data) {
		return "", 0, io.ErrUnexpectedEOF
	}

	value := string(data[pos : pos+n])
	pos += n

	if data[pos] != 0 {
		return "", 0, fmt.Errorf(
			"D-Bus string is not NUL terminated",
		)
	}

	return value, pos + 1, nil
}

func readSignature(
	data []byte,
	pos int,
) (string, int, error) {
	if pos >= len(data) {
		return "", 0, io.ErrUnexpectedEOF
	}

	n := int(data[pos])
	pos++

	if pos+n+1 > len(data) {
		return "", 0, io.ErrUnexpectedEOF
	}

	value := string(data[pos : pos+n])
	pos += n

	if data[pos] != 0 {
		return "", 0, fmt.Errorf(
			"D-Bus signature is not NUL terminated",
		)
	}

	return value, pos + 1, nil
}

func align(pos, alignment int) int {
	return (pos + alignment - 1) & ^(alignment - 1)
}

// -----------------------------------------------------------------------------
// Hello reply
// -----------------------------------------------------------------------------

func writeHelloReply(
	w io.Writer,
	replySerial uint32,
) error {
	// Hello returns the connection's unique bus name.
	body := encodeString(":1.0")

	var fields []byte

	fields = appendHeaderUint32(
		fields,
		5, // REPLY_SERIAL
		replySerial,
	)

	fields = appendHeaderSignature(
		fields,
		8, // SIGNATURE
		"s",
	)

	return writeMessage(
		w,
		messageTypeMethodReturn,
		1,
		fields,
		body,
	)
}

// -----------------------------------------------------------------------------
// UnknownMethod reply
// -----------------------------------------------------------------------------

func writeUnknownMethodReply(
	w io.Writer,
	replySerial uint32,
	iface string,
	member string,
) error {
	var text string

	switch {
	case iface != "" && member != "":
		text = fmt.Sprintf(
			"Method %s.%s is not implemented by the fake D-Bus server",
			iface,
			member,
		)

	case member != "":
		text = fmt.Sprintf(
			"Method %s is not implemented by the fake D-Bus server",
			member,
		)

	default:
		text = "Method is not implemented by the fake D-Bus server"
	}

	// org.freedesktop.DBus.Error.UnknownMethod carries a single
	// string describing the error.
	body := encodeString(text)

	var fields []byte

	fields = appendHeaderString(
		fields,
		4, // ERROR_NAME
		"org.freedesktop.DBus.Error.UnknownMethod",
	)

	fields = appendHeaderUint32(
		fields,
		5, // REPLY_SERIAL
		replySerial,
	)

	fields = appendHeaderSignature(
		fields,
		8, // SIGNATURE
		"s",
	)

	// We use monotonically irrelevant but valid non-zero serials for
	// server replies. There is only one fake peer here.
	return writeMessage(
		w,
		messageTypeError,
		2,
		fields,
		body,
	)
}

// -----------------------------------------------------------------------------
// D-Bus encoder helpers
// -----------------------------------------------------------------------------

func writeMessage(
	w io.Writer,
	msgType byte,
	serial uint32,
	fields []byte,
	body []byte,
) error {
	var msg []byte

	msg = append(msg,
		'l',     // little endian
		msgType, // message type
		0,       // flags
		1,       // D-Bus protocol version
	)

	msg = appendUint32(
		msg,
		uint32(len(body)),
	)

	msg = appendUint32(
		msg,
		serial,
	)

	msg = appendUint32(
		msg,
		uint32(len(fields)),
	)

	msg = append(msg, fields...)

	// Message body starts on an 8-byte boundary.
	for len(msg)%8 != 0 {
		msg = append(msg, 0)
	}

	msg = append(msg, body...)

	_, err := w.Write(msg)
	return err
}

func appendHeaderString(
	fields []byte,
	code byte,
	value string,
) []byte {
	// Header-field struct (yv) is 8-byte aligned.
	for len(fields)%8 != 0 {
		fields = append(fields, 0)
	}

	fields = append(fields, code)

	// Variant contains a STRING.
	fields = append(fields,
		1,
		's',
		0,
	)

	// STRING is 4-byte aligned.
	for len(fields)%4 != 0 {
		fields = append(fields, 0)
	}

	return appendString(fields, value)
}

func appendHeaderUint32(
	fields []byte,
	code byte,
	value uint32,
) []byte {
	for len(fields)%8 != 0 {
		fields = append(fields, 0)
	}

	fields = append(fields, code)

	// Variant contains UINT32.
	fields = append(fields,
		1,
		'u',
		0,
	)

	for len(fields)%4 != 0 {
		fields = append(fields, 0)
	}

	return appendUint32(fields, value)
}

func appendHeaderSignature(
	fields []byte,
	code byte,
	value string,
) []byte {
	for len(fields)%8 != 0 {
		fields = append(fields, 0)
	}

	fields = append(fields, code)

	// Variant contains a SIGNATURE.
	fields = append(fields,
		1,
		'g',
		0,
	)

	fields = append(
		fields,
		byte(len(value)),
	)

	fields = append(fields, value...)
	fields = append(fields, 0)

	return fields
}

func encodeString(s string) []byte {
	return appendString(nil, s)
}

func appendString(
	b []byte,
	s string,
) []byte {
	b = appendUint32(
		b,
		uint32(len(s)),
	)

	b = append(b, s...)
	b = append(b, 0)

	return b
}

func appendUint32(
	b []byte,
	value uint32,
) []byte {
	var v [4]byte

	binary.LittleEndian.PutUint32(
		v[:],
		value,
	)

	return append(b, v[:]...)
}

func setBusAddress(t *testing.T, socketPath string) {
	t.Helper()
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", "unix:path="+socketPath)
	t.Setenv("DBUS_LAUNCHD_SESSION_BUS_SOCKET", socketPath)
}

func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "migp")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}

func expectTimeout(t *testing.T, mgr *Manager, err error) {
	if err == nil {
		_ = mgr.Close()
		t.Fatal("expected an error connecting to an unresponsive D-Bus socket, got nil")
	}
	if mgr != nil {
		t.Errorf("expected a nil manager on error, got %v", mgr)
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected a timeout error, got: %v", err)
	}
}

func TestNewManagerWithTimeoutUnresponsiveSocket(t *testing.T) {
	socketPath := shortSocketPath(t)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to listen on unix socket %q: %v", socketPath, err)
	}
	defer listener.Close()

	// Accept connections but never write a reply, mimicking a bind-mounted host
	// socket with no systemd/D-Bus daemon behind it.
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// Hold the connection open without responding.
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()

	setBusAddress(t, socketPath)

	const timeout = 300 * time.Millisecond
	start := time.Now()
	mgr, err := newManagerWithTimeout(context.Background(), timeout, nil)
	elapsed := time.Since(start)

	expectTimeout(t, mgr, err)

	// The call must return promptly after the timeout, not hang. A generous
	// upper bound still catches a regression back to the indefinite block.
	if elapsed > 5*time.Second {
		t.Errorf("connect took %s, expected it to fail fast near the %s timeout", elapsed, timeout)
	}
}

func TestNewManagerWithTimeoutMissingSocket(t *testing.T) {
	setBusAddress(t, filepath.Join(t.TempDir(), "does-not-exist"))

	const timeout = 10 * time.Second
	start := time.Now()
	mgr, err := newManagerWithTimeout(context.Background(), timeout, nil)
	elapsed := time.Since(start)

	if err == nil {
		_ = mgr.Close()
		t.Fatal("expected an error connecting to a missing D-Bus socket, got nil")
	}
	if mgr != nil {
		t.Errorf("expected a nil manager on error, got %v", mgr)
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected the immediate dial error, not the timeout error: %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("connect to a missing socket took %s, expected it to fail immediately", elapsed)
	}
}

func TestManagerCloseNil(t *testing.T) {
	var mgr Manager
	if err := mgr.Close(); err != nil {
		t.Errorf("Close on a zero-value Manager returned an error: %v", err)
	}
}

func TestManagerRace(t *testing.T) {
	if os.Getenv("WITH_SHUTDOWN_HOST_GPU_CLIENTS") == "false" {
		t.Skip()
	}

	prepareFakeDBus(t)

	const timeout = 1 * time.Second

	var conn *dbus.Conn

	delayedConnect := func(ctx context.Context) (*dbus.Conn, error) {
		var err error

		// Intercept the connection so we can check its state afterwards
		conn, err = dbus.NewSystemConnectionContext(ctx)

		// Only send result after context is canceled to simulate race condition
		<-ctx.Done()

		return conn, err
	}

	mgr, err := newManagerWithTimeout(context.Background(), timeout, delayedConnect)

	expectTimeout(t, mgr, err)

	if conn.Connected() {
		t.Errorf("expected connection to be dropped after timeout")
	}
}

func TestCancelDbus(t *testing.T) {
	if os.Getenv("WITH_SHUTDOWN_HOST_GPU_CLIENTS") == "false" {
		t.Skip()
	}

	prepareFakeDBus(t)

	ctx, cancel := context.WithCancel(context.Background())

	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		t.Errorf("expected successful connection, got error: %v", err)
	}

	if !conn.Connected() {
		t.Error("connection is not connected")
	}

	cancel()

	if conn.Connected() {
		t.Error("canceling connection context did not disconnect dbus")
	}
}

func prepareFakeDBus(t *testing.T) {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "dbus.sock")
	setBusAddress(t, socket)

	server, err := Start(socket)
	if err != nil {
		t.Fatalf("start fake dbus: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("close fake dbus: %v", err)
		}
	})
}

func TestFakeDBus(t *testing.T) {
	prepareFakeDBus(t)

	conn, err := dbus.NewSystemConnectionContext(context.Background())
	if err != nil {
		t.Fatalf("NewSystemConnectionContext(): %v", err)
	}

	if !conn.Connected() {
		t.Error("connection is not connected")
	}

	t.Cleanup(func() {
		conn.Close()
	})
}
