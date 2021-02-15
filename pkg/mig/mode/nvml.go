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

package mode

import (
	"fmt"

	"github.com/NVIDIA/mig-parted/internal/nvml"
	log "github.com/sirupsen/logrus"
)

type nvmlMigModeManager struct {
	nvml nvml.Interface
}

var _ Manager = (*nvmlMigModeManager)(nil)

func tryNvmlShutdown(nvmlLib nvml.Interface) {
	ret := nvmlLib.Shutdown()
	if ret.Value() != nvml.SUCCESS {
		log.Warnf("error shutting down NVML: %v", ret)
	}
}

func NewNvmlMigModeManager() Manager {
	return &nvmlMigModeManager{nvml.New()}
}

func (m *nvmlMigModeManager) IsMigCapable(gpu int) (bool, error) {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return false, fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
	if ret.Value() != nvml.SUCCESS {
		return false, fmt.Errorf("error getting device handle: %v", ret)
	}

	_, _, ret = device.GetMigMode()
	if ret.Value() == nvml.ERROR_NOT_SUPPORTED {
		return false, nil
	}
	if ret.Value() != nvml.SUCCESS {
		return false, fmt.Errorf("error getting Mig mode: %v", ret)
	}

	return true, nil
}

func (m *nvmlMigModeManager) GetMigMode(gpu int) (MigMode, error) {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return -1, fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
	if ret.Value() != nvml.SUCCESS {
		return -1, fmt.Errorf("error getting device handle: %v", ret)
	}

	current, _, ret := device.GetMigMode()
	if ret.Value() != nvml.SUCCESS {
		return -1, fmt.Errorf("error getting Mig mode settings: %v", ret)
	}

	switch current {
	case nvml.DEVICE_MIG_ENABLE:
		return Enabled, nil
	case nvml.DEVICE_MIG_DISABLE:
		return Disabled, nil
	}

	return -1, fmt.Errorf("unknown Mig mode returned by NVML: %v", current)
}

func (m *nvmlMigModeManager) SetMigMode(gpu int, mode MigMode) error {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
	if ret.Value() != nvml.SUCCESS {
		return fmt.Errorf("error getting device handle: %v", ret)
	}

	switch mode {
	case Disabled:
		_, ret = device.SetMigMode(nvml.DEVICE_MIG_DISABLE)
	case Enabled:
		_, ret = device.SetMigMode(nvml.DEVICE_MIG_ENABLE)
	default:
		return fmt.Errorf("unknown Mig mode selected: %v", mode)
	}
	if ret.Value() != nvml.SUCCESS {
		return fmt.Errorf("error setting Mig mode: %v", ret)
	}

	return nil
}

func (m *nvmlMigModeManager) IsMigModeChangePending(gpu int) (bool, error) {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return false, fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
	if ret.Value() != nvml.SUCCESS {
		return false, fmt.Errorf("error getting device handle: %v", ret)
	}

	current, pending, ret := device.GetMigMode()
	if ret.Value() != nvml.SUCCESS {
		return false, fmt.Errorf("error getting Mig mode settings: %v", ret)
	}

	if current == pending {
		return false, nil
	}

	return true, nil
}
