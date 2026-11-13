/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStartupRestoresValidatorBeforeConfigSetup(t *testing.T) {
	tests := []struct {
		name        string
		label       string
		failMethod  string
		wantPatches int
		wantError   string
	}{
		{
			name: "restore paused validator then continue startup", label: "paused-for-mig-change",
			wantPatches: 1, wantError: "failed to setup MIG config: failed to read custom config",
		},
		{
			name: "preserve explicitly disabled validator", label: "false",
			wantError: "failed to setup MIG config: failed to read custom config",
		},
		{
			name: "stop startup on node read failure", label: "paused-for-mig-change",
			failMethod: http.MethodGet, wantError: "failed to restore operator-validator",
		},
		{
			name: "stop startup on restoration failure", label: "paused-for-mig-change",
			failMethod: http.MethodPatch, wantPatches: 1, wantError: "failed to restore operator-validator",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patches := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if req.URL.Path != "/api/v1/nodes/test-node" {
					t.Errorf("unexpected API path: %s", req.URL.Path)
				}
				switch req.Method {
				case http.MethodGet:
				case http.MethodPatch:
					patches++
				default:
					t.Errorf("unexpected API method: %s", req.Method)
				}
				if req.Method == tt.failMethod {
					w.WriteHeader(http.StatusForbidden)
					_ = json.NewEncoder(w).Encode(&metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonForbidden, Code: http.StatusForbidden})
					return
				}
				_ = json.NewEncoder(w).Encode(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "test-node", Labels: map[string]string{"nvidia.com/gpu.deploy.operator-validator": tt.label}}})
			}))
			defer server.Close()
			kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")
			config := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: %s
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user: {}
`, server.URL)
			if err := os.WriteFile(kubeconfig, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			oldKubeconfig, oldNode, oldConfig := kubeconfigFlag, nodeNameFlag, configFileFlag
			t.Cleanup(func() {
				kubeconfigFlag, nodeNameFlag, configFileFlag = oldKubeconfig, oldNode, oldConfig
			})
			kubeconfigFlag, nodeNameFlag = kubeconfig, "test-node"
			// Stop startup at config setup without needing a GPU. Successful recovery
			// must reach this error, while failed recovery must return before it.
			configFileFlag = filepath.Join(t.TempDir(), "missing-config.yaml")
			if err := start(context.Background(), nil); err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("startup error = %v, want %q", err, tt.wantError)
			}
			if patches != tt.wantPatches {
				t.Fatalf("recovery patches = %d, want %d", patches, tt.wantPatches)
			}
		})
	}
}
