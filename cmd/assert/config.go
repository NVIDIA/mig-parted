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

	"github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/cmd/util"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
	"github.com/NVIDIA/mig-parted/pkg/types"

	"gitlab.com/nvidia/cloud-native/go-nvlib/pkg/nvpci"
)

func AssertMigConfig(c *Context) error {
	nvidiaModuleLoaded, err := util.IsNvidiaModuleLoaded()
	if err != nil {
		return fmt.Errorf("error checking if nvidia module loaded: %v", err)
	}

	manager := util.NewCombinedMigManager()

	nvpci := nvpci.New()
	gpus, err := nvpci.GetGPUs()
	if err != nil {
		return fmt.Errorf("error enumerating GPUs: %v", err)
	}

	matched := make([]bool, len(gpus))
	err = WalkSelectedMigConfigForEachGPU(c.MigConfig, func(mc *v1.MigConfigSpec, i int, d types.DeviceID) error {
		capable, err := manager.IsMigCapable(i)
		if err != nil {
			return fmt.Errorf("error checking MIG capable: %v", err)
		}

		if !capable && !mc.MigEnabled {
			matched[i] = true
			return nil
		}

		m, err := manager.GetMigMode(i)
		if err != nil {
			return fmt.Errorf("error getting MIG mode: %v", err)
		}

		if !mc.MigEnabled && m == mode.Disabled {
			matched[i] = true
			return nil
		}

		if !nvidiaModuleLoaded {
			return fmt.Errorf("nvidia module required to assert MIG device configuration: %v", err)
		}

		current, err := manager.GetMigConfig(i)
		if err != nil {
			return fmt.Errorf("error getting MIGConfig: %v", err)
		}

		log.Debugf("    Asserting MIG config: %v", mc.MigDevices)

		if current.Equals(mc.MigDevices) {
			matched[i] = true
			return nil
		}

		matched[i] = false
		return nil
	})

	if err != nil {
		return err
	}

	if util.CountTrue(matched) != len(gpus) {
		return fmt.Errorf("not all GPUs match the specified config")
	}

	return nil
}
