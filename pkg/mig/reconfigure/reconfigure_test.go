/*
 * Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
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

package reconfigure

import (
	"context"
	"path/filepath"
	"testing"
)

// TestNewDoesNotConnectSystemd pins the core of the systemd-less-host fix:
// constructing a Reconfigure must not dial the host's systemd D-Bus. On hosts
// where the system bus socket exists but nothing answers (e.g. Talos Linux),
// an eager connection here would block forever, so the connection has to be
// established lazily instead.
func TestNewDoesNotConnectSystemd(t *testing.T) {
	opts := &Options{
		NodeName:          "test-node",
		SelectedMigConfig: "all-disabled",
	}

	r, err := New(context.Background(), nil, []string{"nvidia-mig-parted"}, opts)
	if err != nil {
		t.Fatalf("New returned an unexpected error: %v", err)
	}
	if r == nil {
		t.Fatal("New returned a nil Reconfigure")
	}
	if r.systemdManager != nil {
		t.Error("New must not establish a systemd D-Bus connection; systemdManager should be nil")
	}
}

// TestSystemdMgrErrorIsNotCached verifies that when the lazy systemd connection
// fails, the error is surfaced and nothing is cached, so a later call can try
// again rather than reusing a broken (nil) manager.
func TestSystemdMgrErrorIsNotCached(t *testing.T) {
	// Point D-Bus at a socket that does not exist so the dial fails immediately
	// (the missing-socket path returns fast, well before the connect timeout).
	missing := "unix:path=" + filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", missing)
	t.Setenv("DBUS_LAUNCHD_SESSION_BUS_SOCKET", filepath.Join(t.TempDir(), "does-not-exist"))

	r := &Reconfigure{
		ctx:  context.Background(),
		opts: &Options{},
	}

	mgr, err := r.systemdMgr()
	if err == nil {
		t.Fatal("expected an error when the systemd D-Bus socket is unavailable, got nil")
	}
	if mgr != nil {
		t.Errorf("expected a nil manager on error, got %v", mgr)
	}
	if r.systemdManager != nil {
		t.Error("a failed connection must not be cached; systemdManager should remain nil")
	}
}

// TestCleanupWithNilManager ensures the deferred cleanup is safe when a
// reconfiguration completes without ever needing systemd (the common
// systemd-less-host path, where the manager is never created).
func TestCleanupWithNilManager(t *testing.T) {
	r := &Reconfigure{}
	r.cleanup() // must not panic
}

func TestMaybeSetPaused(t *testing.T) {
	reconfigure := &Reconfigure{}

	tests := []struct {
		input    string
		expected string
	}{
		{"false", "false"},
		{"true", "paused-for-mig-change"},
		{"paused-for-mig-change", "paused-for-mig-change"},
		{"", "paused-for-mig-change"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := reconfigure.maybeSetPaused(tt.input)
			if result != tt.expected {
				t.Errorf("maybeSetPaused(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMaybeSetTrue(t *testing.T) {
	reconfigure := &Reconfigure{}

	tests := []struct {
		input    string
		expected string
	}{
		{"false", "false"},
		{"true", "true"},
		{"paused-for-mig-change", "true"},
		{"", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := reconfigure.maybeSetTrue(tt.input)
			if result != tt.expected {
				t.Errorf("maybeSetTrue(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}
