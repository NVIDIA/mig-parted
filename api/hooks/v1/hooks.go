/*
 * Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
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

package v1

import (
	"fmt"
	"os"
	"os/exec"
)

// Version indicates the version of the 'Spec' struct used to hold 'Hooks' information.
const Version = "v1"

// Spec is a versioned struct used to hold 'Hooks' information.
type Spec struct {
	Version string   `json:"version"`
	Hooks   HooksMap `json:"hooks"`
}

// HookSpec holds the actual data associated with a runnable Hook.
type HookSpec struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Envs    EnvsMap  `json:"envs"`
	Workdir string   `json:"workdir"`
}

// EnvsMap holds the (key, value) pairs associated with a set of environment variables.
type EnvsMap map[string]string

// HooksMap holds (key, value) pairs mapping a list of HookSpec's to a named hook.
type HooksMap map[string][]HookSpec

// Run executes all of the hooks associated with a given name in the HooksMap.
// It injects the environment variables associated with the provided EnvMap,
// and optionally prints the output for each hook to stdout and stderr.
func (h HooksMap) Run(name string, envs EnvsMap, output bool) error {
	hooks, exists := h[name]
	if !exists {
		return nil
	}
	for _, hook := range hooks {
		err := hook.Run(envs, output)
		if err != nil {
			return err
		}
	}
	return nil
}

// Run executes a specific hook from a HookSpec.
// It injects the environment variables associated with the provided EnvMap,
// and optionally prints the output for each hook to stdout and stderr.
func (h *HookSpec) Run(envs EnvsMap, output bool) error {
	cmd := exec.Command(h.Command, h.Args...) //nolint:gosec
	cmd.Env = h.Envs.Combine(envs).Format()
	cmd.Dir = h.Workdir
	if output {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

// Combine merges to EnvMaps together
// Overlapping environment variables from e2 take precedence over those in e.
func (e EnvsMap) Combine(e2 EnvsMap) EnvsMap {
	combined := make(EnvsMap)
	for k, v := range e {
		combined[k] = v
	}
	for k, v := range e2 {
		combined[k] = v
	}
	return combined
}

// Format converts an EnvMap into a list of strings, where each entry is of the form "key=value".
func (e EnvsMap) Format() []string {
	var envs []string
	for k, v := range e {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}
	return envs
}
