/*
 * Copyright (c) 2022, NVIDIA CORPORATION.  All rights reserved.
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

package types

import (
	"github.com/NVIDIA/mig-parted/internal/nvlib/mig"
	"github.com/NVIDIA/mig-parted/internal/nvml"
)

// MigState stores the MIG state for a set of GPUs.
type MigState struct {
	Devices []DeviceState
}

// DeviceState stores the MIG state for a specific GPU.
type DeviceState struct {
	UUID         string
	MigMode      mig.Mode
	GpuInstances []GpuInstanceState
}

// GpuInstanceState stores the MIG state for a specific GPUInstance.
type GpuInstanceState struct {
	ProfileID        int
	Placement        nvml.GpuInstancePlacement
	ComputeInstances []ComputeInstanceState
}

// ComputeInstanceState stores the MIG state for a specific ComputeInstance.
type ComputeInstanceState struct {
	ProfileID    int
	EngProfileID int
}
