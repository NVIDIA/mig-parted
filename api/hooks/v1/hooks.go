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

const Version = "v1"

type Spec struct {
	Version string   `json:"version"`
	Hooks   HooksMap `json:"hooks"`
}

type HookSpec struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Envs    EnvsMap  `json:"envs"`
	Workdir string   `json:"workdir"`
}

type EnvsMap map[string]string
type HooksMap map[string][]HookSpec

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

func (h *HookSpec) Run(envs EnvsMap, output bool) error {
	cmd := exec.Command(h.Command, h.Args...)
	cmd.Env = h.Envs.Combine(envs).Format()
	cmd.Dir = h.Workdir
	if output {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

func (e1 EnvsMap) Combine(e2 EnvsMap) EnvsMap {
	combined := make(EnvsMap)
	for k, v := range e1 {
		combined[k] = v
	}
	for k, v := range e2 {
		combined[k] = v
	}
	return combined
}

func (e EnvsMap) Format() []string {
	var envs []string
	for k, v := range e {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}
	return envs
}
