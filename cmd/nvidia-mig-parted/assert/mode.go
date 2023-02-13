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

package assert

import (
	"fmt"

	v1 "github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/util"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

func AssertMigMode(c *Context) error {
	nvidiaModuleLoaded, err := util.IsNvidiaModuleLoaded()
	if err != nil {
		return fmt.Errorf("error checking if nvidia module loaded: %v", err)
	}

	if nvidiaModuleLoaded {
		err := util.NvmlInit(c.Nvml)
		if err != nil {
			return fmt.Errorf("error initializing NVML: %v", err)
		}
		defer util.TryNvmlShutdown(c.Nvml)
	}

	return WalkSelectedMigConfigForEachGPU(c.MigConfig, func(mc *v1.MigConfigSpec, i int, d types.DeviceID) error {
		if mc.MigEnabled {
			log.Debugf("    Asserting MIG mode: %v", mode.Enabled)
		} else {
			log.Debugf("    Asserting MIG mode: %v", mode.Disabled)
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

		if !capable && mc.MigEnabled {
			return fmt.Errorf("unable to assert MIG mode enabled on non MIG-capable GPU")
		}

		if !capable && !mc.MigEnabled {
			return nil
		}

		m, err := manager.GetMigMode(i)
		if err != nil {
			return fmt.Errorf("error getting MIG mode: %v", err)
		}
		log.Debugf("    Current MIG mode: %v", m)

		if mc.MigEnabled && m == mode.Disabled {
			return fmt.Errorf("current mode different than mode being asserted")
		}
		if !mc.MigEnabled && m == mode.Enabled {
			return fmt.Errorf("current mode different than mode being asserted")
		}

		return nil
	})
}
