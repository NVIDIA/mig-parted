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
	"fmt"

	v1 "github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/assert"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/util"
	"github.com/NVIDIA/mig-parted/pkg/mig/config"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
	"github.com/NVIDIA/mig-parted/pkg/types"

	"gitlab.com/nvidia/cloud-native/go-nvlib/pkg/nvpci"
)

func ApplyMigMode(c *Context) error {
	nvidiaModuleLoaded, err := util.IsNvidiaModuleLoaded()
	if err != nil {
		return fmt.Errorf("error checking if nvidia module loaded: %v", err)
	}

	if nvidiaModuleLoaded {
		err := util.NvmlInit(c.Nvml)
		if err != nil {
			return fmt.Errorf("error initializing NVML: %v", err)
		}
	}

	nvpci := nvpci.New()
	gpus, err := nvpci.GetGPUs()
	if err != nil {
		return fmt.Errorf("error enumerating GPUs: %v", err)
	}

	pending := make([]bool, len(gpus))
	err = assert.WalkSelectedMigConfigForEachGPU(c.MigConfig, func(mc *v1.MigConfigSpec, i int, d types.DeviceID) error {
		desiredMode := mode.Disabled
		if mc.MigEnabled {
			desiredMode = mode.Enabled
		}

		manager, err := util.NewMigModeManager()
		if err != nil {
			return fmt.Errorf("error creating MIG mode Manager: %v", err)
		}

		capable, err := manager.IsMigCapable(i)
		if err != nil {
			return fmt.Errorf("error checking MIG capable: %v", err)
		}
		log.Debugf("    MIG capable: %v\n", capable)

		if !capable && mc.MatchesAllDevices() {
			log.Debugf("    Skipping -- non MIG-capable GPU")
			return nil
		}

		if !capable && !mc.MigEnabled {
			log.Debugf("    Skipping -- non MIG-capable GPU with MIG mode disabled")
			return nil
		}

		if !capable && mc.MigEnabled {
			return fmt.Errorf("cannot set MIG mode on non MIG-capable GPU")
		}

		currentMode, err := manager.GetMigMode(i)
		if err != nil {
			return fmt.Errorf("error getting MIG mode: %v", err)
		}
		log.Debugf("    Current MIG mode: %v", currentMode)

		if nvidiaModuleLoaded && currentMode != mode.Disabled {
			log.Debugf("    Clearing existing MIG configuration")
			manager := config.NewNvmlMigConfigManager()
			err := manager.ClearMigConfig(i)
			if err != nil {
				return fmt.Errorf("error clearing existing MIG configurations: %v", err)
			}
		}

		log.Debugf("    Updating MIG mode: %v", desiredMode)
		err = manager.SetMigMode(i, desiredMode)
		if err != nil {
			return fmt.Errorf("error setting MIG mode: %v", err)
		}

		pending[i], err = manager.IsMigModeChangePending(i)
		if err != nil {
			return fmt.Errorf("error checking pending MIG mode change: %v", err)
		}
		log.Debugf("    Mode change pending: %v", pending[i])

		return nil
	})

	if nvidiaModuleLoaded {
		util.TryNvmlShutdown(c.Nvml)
	}

	if err != nil {
		return err
	}

	if c.Flags.SkipReset || !util.Any(pending) {
		return nil
	}

	log.Debugf("At least one mode change pending")
	log.Debugf("Resetting GPUs...")

	if nvidiaModuleLoaded {
		log.Debugf("  NVIDIA kernel module loaded")
		log.Debugf("  Using nvidia-smi to perform GPU reset")
		var pci []string
		for _, gpu := range gpus {
			if gpu.Is3DController() {
				pci = append(pci, gpu.Address)
			}
		}
		output, err := util.NvidiaSmiReset(pci...)
		if err != nil {
			log.Errorf("%v", output)
			return fmt.Errorf("error resetting all GPUs: %v", err)
		}
	} else {
		log.Debugf("  No NVIDIA kernel module loaded")
		log.Debugf("  Using PCIe to perform GPU reset")
		for i, gpu := range gpus {
			if pending[i] {
				err = gpu.Reset()
				if err != nil {
					return fmt.Errorf("error resetting GPU %v: %v", i, err)
				}
			}
		}
	}

	return nil
}
