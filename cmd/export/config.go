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

package export

import (
	"fmt"

	"github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/cmd/util"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
	"github.com/NVIDIA/mig-parted/pkg/nvpci"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

func ExportMigConfigs(c *Context) (*v1.Spec, error) {
	nvidiaModuleLoaded, err := util.IsNvidiaModuleLoaded()
	if err != nil {
		return nil, fmt.Errorf("error checking if nvidia module loaded: %v", err)
	}

	nvpci := nvpci.New()
	gpus, err := nvpci.GetGPUs()
	if err != nil {
		return nil, fmt.Errorf("error enumerating GPUs: %v", err)
	}

	manager := util.NewCombinedMigManager()

	configSpecs := make(v1.MigConfigSpecSlice, len(gpus))
	for i, gpu := range gpus {
		deviceID := types.NewDeviceID(gpu.Device, gpu.Vendor)
		deviceFilter := deviceID.String()

		enabled := false
		capable, err := manager.IsMigCapable(i)
		if err != nil {
			return nil, fmt.Errorf("error checking MIG capable: %v", err)
		}
		if capable {
			m, err := manager.GetMigMode(i)
			if err != nil {
				return nil, fmt.Errorf("error checking MIG capable: %v", err)
			}
			enabled = (m == mode.Enabled)
		}

		migDevices := types.MigConfig{}
		if enabled {
			if !nvidiaModuleLoaded {
				return nil, fmt.Errorf("nvidia module must be loaded in order to query MIG device state")
			}

			migDevices, err = manager.GetMigConfig(i)
			if err != nil {
				return nil, fmt.Errorf("error getting MIGConfig: %v", err)
			}
		}

		configSpecs[i] = v1.MigConfigSpec{
			DeviceFilter: deviceFilter,
			Devices:      []int{i},
			MigEnabled:   enabled,
			MigDevices:   migDevices,
		}
	}

	spec := v1.Spec{
		Version: v1.Version,
		MigConfigs: map[string]v1.MigConfigSpecSlice{
			c.Flags.ConfigLabel: configSpecs.Normalize(),
		},
	}

	return &spec, nil
}
