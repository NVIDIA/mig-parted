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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

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

func TestSystemdMgrErrorIsNotCached(t *testing.T) {
	missing := "unix:path=" + filepath.Join(t.TempDir(), "does-not-exist")
	t.Setenv("DBUS_SYSTEM_BUS_ADDRESS", missing)
	t.Setenv("DBUS_LAUNCHD_SESSION_BUS_SOCKET", filepath.Join(t.TempDir(), "does-not-exist"))

	r := &Reconfigure{
		ctx:  context.Background(),
		opts: &Options{},
	}

	mgr, err := r.getSystemdManager()
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

// Use an API server stub so deletion acknowledgement and process termination
// can be controlled independently, unlike an immediately deleting object tracker.
func testClient(t *testing.T, handler http.HandlerFunc) *kubernetes.Clientset {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL, QPS: -1})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestValidatorDeploymentLabels(t *testing.T) {
	tests := []struct {
		initial      string
		wantPaused   string
		wantRestored string
	}{
		{"true", validatorPaused, "true"},
		{"false", "false", "false"},
		{"", "", ""},
		{validatorPaused, validatorPaused, "true"},
	}
	for _, tt := range tests {
		t.Run(tt.initial, func(t *testing.T) {
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", ResourceVersion: "7", Labels: map[string]string{}}}
			if tt.initial != "" {
				node.Labels[operatorValidatorDeployLabel] = tt.initial
			}
			client := testClient(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.Method == http.MethodPatch {
					var patch struct {
						Metadata struct{ Labels map[string]string }
					}
					if err := json.NewDecoder(req.Body).Decode(&patch); err != nil {
						t.Error(err)
					}
					for k, v := range patch.Metadata.Labels {
						node.Labels[k] = v
					}
				}
				_ = json.NewEncoder(w).Encode(node)
			})
			r, _ := New(context.Background(), client, nil, &Options{NodeName: "node"})
			if err := r.getCurrentNodeLabels(); err != nil {
				t.Fatal(err)
			}
			if err := r.shutdownKubernetesGPUClients(); err != nil {
				t.Fatal(err)
			}
			got, _ := client.CoreV1().Nodes().Get(r.ctx, "node", metav1.GetOptions{})
			if got.Labels[operatorValidatorDeployLabel] != tt.wantPaused {
				t.Fatalf("paused label = %q, want %q", got.Labels[operatorValidatorDeployLabel], tt.wantPaused)
			}
			previous := NoRestartK8sDaemonsetsOnExit
			NoRestartK8sDaemonsetsOnExit = true
			defer func() { NoRestartK8sDaemonsetsOnExit = previous }()
			if err := r.setState(migStateFailed); err != nil {
				t.Fatal(err)
			}
			got, _ = client.CoreV1().Nodes().Get(r.ctx, "node", metav1.GetOptions{})
			if got.Labels[operatorValidatorDeployLabel] != tt.wantRestored {
				t.Fatalf("failure restored label = %q, want %q", got.Labels[operatorValidatorDeployLabel], tt.wantRestored)
			}
			if got.Labels[migConfigStateLabel] != migStateFailed {
				t.Fatal("missing failed state")
			}
			if err := r.shutdownKubernetesGPUClients(); err != nil {
				t.Fatal(err)
			}
			if err := r.restartKubernetesGPUClients(); err != nil {
				t.Fatal(err)
			}
			if err := r.setState(migStateSuccess); err != nil {
				t.Fatal(err)
			}
			got, _ = client.CoreV1().Nodes().Get(r.ctx, "node", metav1.GetOptions{})
			if got.Labels[operatorValidatorDeployLabel] != tt.wantRestored {
				t.Fatalf("success restored label = %q, want %q", got.Labels[operatorValidatorDeployLabel], tt.wantRestored)
			}
		})
	}
}

func TestRestoreValidator(t *testing.T) {
	tests := []struct {
		name  string
		label string
		// If set, patches fail with a conflict and later reads return this label.
		labelAfterConflict string
		wantPatches        int
	}{
		{name: "enabled", label: "true"},
		{name: "disabled", label: "false"},
		{name: "unset", label: ""},
		{name: "custom", label: "custom"},
		{name: "paused", label: validatorPaused, wantPatches: 1},
		{name: "concurrent disable", label: validatorPaused, labelAfterConflict: "false", wantPatches: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := 0
			client := testClient(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.Method == http.MethodPatch {
					patches++
					var patch struct {
						Metadata struct {
							ResourceVersion string
							Labels          map[string]string
						}
					}
					if err := json.NewDecoder(req.Body).Decode(&patch); err != nil {
						t.Error(err)
					}
					if patch.Metadata.ResourceVersion != "7" || patch.Metadata.Labels[operatorValidatorDeployLabel] != "true" {
						t.Errorf("unexpected recovery patch: %+v", patch)
					}
					if len(patch.Metadata.Labels) != 1 {
						t.Error("recovery changed other labels")
					}
					if tt.labelAfterConflict != "" {
						w.WriteHeader(http.StatusConflict)
						_ = json.NewEncoder(w).Encode(&metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonConflict, Code: http.StatusConflict})
						return
					}
				}
				label := tt.label
				if tt.labelAfterConflict != "" && patches > 0 {
					label = tt.labelAfterConflict
				}
				_ = json.NewEncoder(w).Encode(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", ResourceVersion: "7", Labels: map[string]string{operatorValidatorDeployLabel: label}}})
			})
			if err := RestoreValidator(context.Background(), client, "node"); err != nil {
				t.Fatal(err)
			}
			if patches != tt.wantPatches {
				t.Fatalf("patches = %d, want %d", patches, tt.wantPatches)
			}
		})
	}
}

func TestDrainValidatorWorkloadsAfterCreatorStops(t *testing.T) {
	creatorStopped := false
	deleted := map[string]bool{}
	drained := map[string]bool{}
	client := testClient(t, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method == http.MethodDelete {
			if !creatorStopped {
				t.Error("deleting workload before creator stopped")
			}
			deleted[filepath.Base(req.URL.Path)] = true
			_ = json.NewEncoder(w).Encode(&metav1.Status{Status: metav1.StatusSuccess})
			return
		}
		selector := req.URL.Query().Get("labelSelector")
		pods := &corev1.PodList{}
		switch selector {
		case "app=nvidia-operator-validator":
			// Model a workload created as the creator finishes terminating.
			creatorStopped = true
		case "app=nvidia-cuda-validator", "app=nvidia-device-plugin-validator":
			if !creatorStopped {
				t.Error("inspecting workloads before creator stopped")
			}
			app := selector[len("app="):]
			if deleted[app] {
				drained[app] = true
			} else {
				pods.Items = []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: app, Namespace: "operator"}}}
			}
		}
		_ = json.NewEncoder(w).Encode(pods)
	})
	r, _ := New(context.Background(), client, nil, &Options{NodeName: "node", DefaultGPUClientsNamespace: "operator"})
	if err := r.waitForPodsToBeDeleted(); err != nil {
		t.Fatal(err)
	}
	for _, app := range []string{"nvidia-cuda-validator", "nvidia-device-plugin-validator"} {
		if !deleted[app] || !drained[app] {
			t.Fatalf("%s was not deleted and drained", app)
		}
	}
}

func TestDrainWaitsForTermination(t *testing.T) {
	tests := []struct {
		app         string
		wantDeleted bool
	}{
		// The DaemonSet stops the creator, so the drain must not delete it.
		{"nvidia-operator-validator", false},
		{"nvidia-cuda-validator", true},
		{"nvidia-device-plugin-validator", true},
	}
	for _, tt := range tests {
		app := tt.app
		t.Run(app, func(t *testing.T) {
			deleted := false
			client := testClient(t, func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.Method == http.MethodDelete {
					deleted = true
					_ = json.NewEncoder(w).Encode(&metav1.Status{Status: metav1.StatusSuccess})
					return
				}
				pods := &corev1.PodList{}
				if req.URL.Query().Get("labelSelector") == "app="+app {
					pods.Items = []corev1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: app, Namespace: "operator", Finalizers: []string{"test/hold"}}}}
				}
				_ = json.NewEncoder(w).Encode(pods)
			})
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			r, _ := New(ctx, client, nil, &Options{NodeName: "node", DefaultGPUClientsNamespace: "operator"})
			err := r.waitForPodsToBeDeleted()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected termination wait to time out, got %v", err)
			}
			if deleted != tt.wantDeleted {
				t.Fatalf("deletion requested = %v, want %v", deleted, tt.wantDeleted)
			}
		})
	}
}
