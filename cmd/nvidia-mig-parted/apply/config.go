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

	"github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/cmd/assert"
	"github.com/NVIDIA/mig-parted/cmd/util"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

func ApplyMigConfig(c *Context) error {
	err := util.NvmlInit(c.Nvml)
	if err != nil {
		return fmt.Errorf("error initializing NVML: %v", err)
	}
	defer util.TryNvmlShutdown(c.Nvml)

	return assert.WalkSelectedMigConfigForEachGPU(c.MigConfig, func(mc *v1.MigConfigSpec, i int, d types.DeviceID) error {
		modeManager, err := util.NewMigModeManager()
		if err != nil {
			return fmt.Errorf("error creating MIG mode Manager: %v", err)
		}

		capable, err := modeManager.IsMigCapable(i)
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
			return fmt.Errorf("cannot set MIG config on non MIG-capable GPU")
		}

		m, err := modeManager.GetMigMode(i)
		if err != nil {
			return fmt.Errorf("error getting MIG mode: %v", err)
		}

		if mc.MigEnabled && m == mode.Disabled {
			return fmt.Errorf("unable to apply MIG config with MIG mode disabled")
		}

		if !mc.MigEnabled && m == mode.Enabled {
			return fmt.Errorf("MIG mode is currently enabled, but the configuration specifies it should be disabled")
		}

		if !mc.MigEnabled {
			log.Debugf("    Skipping MIG config -- MIG disabled")
			return nil
		}

		configManager, err := util.NewMigConfigManager()
		if err != nil {
			return fmt.Errorf("error creating MIG config Manager: %v", err)
		}

		current, err := configManager.GetMigConfig(i)
		if err != nil {
			return fmt.Errorf("error getting MIGConfig: %v", err)
		}

		log.Debugf("    Updating MIG config: %v", mc.MigDevices)

		if current.Equals(mc.MigDevices) {
			log.Debugf("    Skipping -- already set to desired value")
			return nil
		}

		err = configManager.SetMigConfig(i, mc.MigDevices)
		if err != nil {
			return fmt.Errorf("error setting MIGConfig: %v", err)
		}

		return nil
	})
}
