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
	"sort"

	"github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/cmd/util"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
	"github.com/NVIDIA/mig-parted/pkg/types"

	"gitlab.com/nvidia/cloud-native/go-nvlib/pkg/nvpci"
)

func ExportMigConfigs(c *Context) (*v1.Spec, error) {
	err := util.NvmlInit(c.Nvml)
	if err != nil {
		return nil, fmt.Errorf("error initializing NVML: %v", err)
	}
	defer util.TryNvmlShutdown(c.Nvml)

	nvpci := nvpci.New()
	gpus, err := nvpci.GetGPUs()
	if err != nil {
		return nil, fmt.Errorf("error enumerating GPUs: %v", err)
	}

	configSpecs := make(v1.MigConfigSpecSlice, len(gpus))
	for i, gpu := range gpus {
		deviceID := types.NewDeviceID(gpu.Device, gpu.Vendor)
		deviceFilter := deviceID.String()

		modeManager, err := util.NewMigModeManager()
		if err != nil {
			return nil, fmt.Errorf("error creating MIG Mode Manager: %v", err)
		}

		enabled := false
		capable, err := modeManager.IsMigCapable(i)
		if err != nil {
			return nil, fmt.Errorf("error checking MIG capable: %v", err)
		}
		if capable {
			m, err := modeManager.GetMigMode(i)
			if err != nil {
				return nil, fmt.Errorf("error checking MIG capable: %v", err)
			}
			enabled = (m == mode.Enabled)
		}

		migDevices := types.MigConfig{}
		if enabled {
			configManager, err := util.NewMigConfigManager()
			if err != nil {
				return nil, fmt.Errorf("error creating MIG Config Manager: %v", err)
			}

			migDevices, err = configManager.GetMigConfig(i)
			if err != nil {
				return nil, fmt.Errorf("error getting MIGConfig: %v", err)
			}
		}

		configSpecs[i] = v1.MigConfigSpec{
			DeviceFilter: []string{deviceFilter},
			Devices:      []int{i},
			MigEnabled:   enabled,
			MigDevices:   migDevices,
		}
	}

	spec := v1.Spec{
		Version: v1.Version,
		MigConfigs: map[string]v1.MigConfigSpecSlice{
			c.Flags.ConfigLabel: mergeMigConfigSpecs(configSpecs),
		},
	}

	return &spec, nil
}

// mergeMigConfigSpecs merges the specs from a MigConfigSpecsSlice into a more
// compact form for better display.
//
// We assume the 'specs' argument is passed in from the ExportMigConfigs(), so we know
// that the 'interface{}' types for '.DeviceFilter' and '.Devices' are both
// slices and not strings.
//
// We also know that every device on the node is represented and that each spec
// has a single device set in '.Devices' and a single filter set in
// '.DeviceFilter'.
//
// This allows us to simplify the logic below significantly.
func mergeMigConfigSpecs(specs v1.MigConfigSpecSlice) v1.MigConfigSpecSlice {
	// Merge the incoming specs by comparing their MigEnabled and MigDevices fields.
	// For any two specs, if both of these are equal, then we merge them
	// together and concatenate their device filter and devices lists.
	merged := []v1.MigConfigSpec{}
OUTER:
	for _, s := range specs {
		if len(merged) == 0 {
			merged = append(merged, s)
			continue
		}
		for i, m := range merged {
			if s.MigEnabled != m.MigEnabled {
				continue
			}
			if !s.MigDevices.Equals(m.MigDevices) {
				continue
			}
			merged[i].Devices = mergeAndSortIntSlices(m.Devices.([]int), s.Devices.([]int))
			merged[i].DeviceFilter = mergeAndSortStringSlices(m.DeviceFilter.([]string), s.DeviceFilter.([]string))
			continue OUTER
		}
		merged = append(merged, s)
	}

	// Get the set of devices per unique device filter.
	// This assumes the incoming MigConfigSpecSlice has
	// a single entry in the device filter for each spec.
	dfDevices := make(map[string][]int)
	for _, s := range specs {
		df := s.DeviceFilter.([]string)[0]
		dfDevices[df] = mergeAndSortIntSlices(dfDevices[df], s.Devices.([]int))
	}

	// Run back over the merged list to see if we can:
	// (1) Change its list of device filters to a single string or nil it out.
	// (2) Convert its list of devices to the special keyword 'all'.
	for i, m := range merged {
		if len(dfDevices) == 1 {
			merged[i].DeviceFilter = nil
		} else if len(m.DeviceFilter.([]string)) == 1 {
			merged[i].DeviceFilter = m.DeviceFilter.([]string)[0]
		}

		var specDevices []int
		for _, df := range m.DeviceFilter.([]string) {
			specDevices = mergeAndSortIntSlices(specDevices, dfDevices[df])
		}
		if !equalSortedIntSlices(m.Devices.([]int), specDevices) {
			continue
		}
		merged[i].Devices = "all"
	}

	// If there is only a single entry in the end,
	// remove the device filter completely.
	if len(merged) == 1 {
		merged[0].DeviceFilter = nil
	}

	return merged
}

func mergeAndSortIntSlices(slices ...[]int) []int {
	set := make(map[int]struct{})
	for _, s := range slices {
		for _, e := range s {
			set[e] = struct{}{}
		}
	}

	var out []int
	for e := range set {
		out = append(out, e)
	}

	sort.Ints(out)
	return out
}

func mergeAndSortStringSlices(slices ...[]string) []string {
	set := make(map[string]struct{})
	for _, s := range slices {
		for _, e := range s {
			set[e] = struct{}{}
		}
	}

	var out []string
	for e := range set {
		out = append(out, e)
	}

	sort.Strings(out)
	return out
}

func equalSortedIntSlices(slices ...[]int) bool {
	if len(slices) < 2 {
		return true
	}

	s0 := slices[0]
	for _, s1 := range slices[1:] {
		if len(s0) != len(s1) {
			return false
		}
		for i := range s0 {
			if s0[i] != s1[i] {
				return false
			}
		}
	}

	return true
}
