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

package apply

import (
	hooks "github.com/NVIDIA/mig-parted/api/hooks/v1"
)

const (
	applyStartHook     = "apply-start"
	preApplyModeHook   = "pre-apply-mode"
	preApplyConfigHook = "pre-apply-config"
	applyExitHook      = "apply-exit"
)

type applyHooks struct {
	hooks.HooksMap
}

type ApplyHooks interface {
	ApplyStart(envs hooks.EnvsMap, output bool) error
	PreApplyMode(envs hooks.EnvsMap, output bool) error
	PreApplyConfig(envs hooks.EnvsMap, output bool) error
	ApplyExit(envs hooks.EnvsMap, output bool) error
}

var _ ApplyHooks = (*applyHooks)(nil)

func NewApplyHooks(hooksMap hooks.HooksMap) ApplyHooks {
	return &applyHooks{hooksMap}
}

func (h *applyHooks) ApplyStart(envs hooks.EnvsMap, output bool) error {
	return h.Run(applyStartHook, envs, output)
}

func (h *applyHooks) PreApplyMode(envs hooks.EnvsMap, output bool) error {
	return h.Run(preApplyModeHook, envs, output)
}

func (h *applyHooks) PreApplyConfig(envs hooks.EnvsMap, output bool) error {
	return h.Run(preApplyConfigHook, envs, output)
}

func (h *applyHooks) ApplyExit(envs hooks.EnvsMap, output bool) error {
	return h.Run(applyExitHook, envs, output)
}
