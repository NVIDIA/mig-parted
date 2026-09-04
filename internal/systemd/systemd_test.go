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
)

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
	mgr, err := newManagerWithTimeout(context.Background(), timeout)
	elapsed := time.Since(start)

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
	mgr, err := newManagerWithTimeout(context.Background(), timeout)
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
