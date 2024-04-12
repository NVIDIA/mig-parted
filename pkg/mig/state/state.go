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

package state

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/NVIDIA/mig-parted/internal/nvlib"
	"github.com/NVIDIA/mig-parted/pkg/mig/config"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

// Manager represents the set of operations for fetching / restoring the full MIG state of all GPUs on a node.
type Manager interface {
	Fetch() (*types.MigState, error)
	RestoreMode(state *types.MigState) error
	RestoreConfig(state *types.MigState) error
}

type migStateManager struct {
	nvml   nvml.Interface
	nvlib  nvlib.Interface
	mode   mode.Manager
	config config.Manager
}

var _ Manager = (*migStateManager)(nil)

func tryNvmlShutdown(nvmlLib nvml.Interface) {
	ret := nvmlLib.Shutdown()
	if ret != nvml.SUCCESS {
		log.Warnf("Error shutting down NVML: %v", ret)
	}
}

// NewMigStateManager creates a new MIG state Manager.
func NewMigStateManager() Manager {
	return &migStateManager{
		nvml.New(),
		nvlib.New(),
		mode.NewNvmlMigModeManager(),
		config.NewNvmlMigConfigManager(),
	}
}

// NewMockMigStateManager creates a MIG state Manager using 'nvml' as the underlying NVML library to mock out its calls.
func NewMockMigStateManager(nvml nvml.Interface) Manager {
	return &migStateManager{
		nvml,
		nvlib.NewMock(nvml),
		mode.NewMockNvmlMigModeManager(nvml),
		config.NewMockNvmlMigConfigManager(nvml),
	}
}

// Fetch collects the full MIG state of all GPUs on a node and returns it in a 'MigState' struct.
func (m *migStateManager) Fetch() (*types.MigState, error) {
	ret := m.nvml.Init()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	numGPUs, ret := m.nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting device count: %v", ret)
	}

	var migState types.MigState
	for gpu := 0; gpu < numGPUs; gpu++ {
		capable, err := m.mode.IsMigCapable(gpu)
		if err != nil {
			return nil, fmt.Errorf("error checking MIG capable: %v", err)
		}

		if !capable {
			continue
		}

		device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("error getting device handle: %v", ret)
		}

		uuid, ret := device.GetUUID()
		if ret != nvml.SUCCESS {
			return nil, fmt.Errorf("error getting device uuid: %v", ret)
		}

		deviceState := types.DeviceState{
			UUID: uuid,
		}

		deviceState.MigMode, err = m.mode.GetMigMode(gpu)
		if err != nil {
			return nil, fmt.Errorf("error getting MIG mode: %v", err)
		}

		if deviceState.MigMode == mode.Disabled {
			migState.Devices = append(migState.Devices, deviceState)
			continue
		}

		err = m.nvlib.Mig.Device(device).WalkGpuInstances(func(gi nvml.GpuInstance, giProfileID int, giProfileInfo nvml.GpuInstanceProfileInfo) error {
			giInfo, ret := gi.GetInfo()
			if ret != nvml.SUCCESS {
				return fmt.Errorf("error getting GPU instance info for '%v': %v", giProfileID, ret)
			}

			giState := types.GpuInstanceState{
				ProfileID: giProfileID,
				Placement: giInfo.Placement,
			}

			err := m.nvlib.Mig.GpuInstance(gi).WalkComputeInstances(func(ci nvml.ComputeInstance, ciProfileID int, ciEngProfileID int, ciProfileInfo nvml.ComputeInstanceProfileInfo) error {
				ciState := types.ComputeInstanceState{
					ProfileID:    ciProfileID,
					EngProfileID: ciEngProfileID,
				}

				giState.ComputeInstances = append(giState.ComputeInstances, ciState)
				return nil
			})
			if err != nil {
				return fmt.Errorf("error walking compute instances for '%v': %v", giProfileID, err)
			}
			deviceState.GpuInstances = append(deviceState.GpuInstances, giState)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("error walking gpu instances for '%v': %v", gpu, err)
		}
		migState.Devices = append(migState.Devices, deviceState)
	}

	return &migState, nil
}

// RestoreMode restores just the MIG mode state of all GPUs represented in the provided 'MigState'.
func (m *migStateManager) RestoreMode(state *types.MigState) error {
	ret := m.nvml.Init()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	for _, deviceState := range state.Devices {
		device, ret := m.nvml.DeviceGetHandleByUUID(deviceState.UUID)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting device handle: %v", ret)
		}

		index, ret := device.GetIndex()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting device index: %v", ret)
		}

		err := m.mode.SetMigMode(index, deviceState.MigMode)
		if err != nil {
			return fmt.Errorf("error setting MIG mode on device '%v': %v", deviceState.UUID, err)
		}
	}

	return nil
}

// RestoreMode restores the full MIG configuration of all GPUs represented in the provided 'MigState'.
func (m *migStateManager) RestoreConfig(state *types.MigState) error {
	ret := m.nvml.Init()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	for _, deviceState := range state.Devices {
		if deviceState.MigMode == mode.Disabled {
			continue
		}

		device, ret := m.nvml.DeviceGetHandleByUUID(deviceState.UUID)
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting device handle: %v", ret)
		}

		index, ret := device.GetIndex()
		if ret != nvml.SUCCESS {
			return fmt.Errorf("error getting device index: %v", ret)
		}

		err := m.config.ClearMigConfig(index)
		if err != nil {
			return fmt.Errorf("error clearing existing MIG config: %v", err)
		}

		for _, giState := range deviceState.GpuInstances {
			giProfileInfo, ret := device.GetGpuInstanceProfileInfo(giState.ProfileID)
			if ret != nvml.SUCCESS {
				return fmt.Errorf("error getting GPU instance profile info for '%v': %v", giState.ProfileID, ret)
			}

			placement := giState.Placement
			gi, ret := device.CreateGpuInstanceWithPlacement(&giProfileInfo, &placement)
			if ret != nvml.SUCCESS {
				return fmt.Errorf("error creating GPU instance for '%v': %v", giState.ProfileID, ret)
			}

			for _, ciState := range giState.ComputeInstances {
				ciProfileInfo, ret := gi.GetComputeInstanceProfileInfo(ciState.ProfileID, ciState.EngProfileID)
				if ret != nvml.SUCCESS {
					return fmt.Errorf("error getting Compute instance profile info for '(%v, %v)': %v", ciState.ProfileID, ciState.EngProfileID, ret)
				}

				_, ret = gi.CreateComputeInstance(&ciProfileInfo)
				if ret != nvml.SUCCESS {
					return fmt.Errorf("error creating Compute instance for '(%v, %v)': %v", ciState.ProfileID, ciState.EngProfileID, ret)
				}
			}
		}
	}

	return nil
}
