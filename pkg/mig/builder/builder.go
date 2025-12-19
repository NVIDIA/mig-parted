/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
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

package builder

import (
	"fmt"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"

	migspec "github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/pkg/mig/discovery"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

// normalizeProfileName converts profile names to config-friendly format
// Replaces '+' with '.' for attributes like +me, +gfx, +me.all
// Examples: "1g.24gb+me" -> "1g.24gb.me", "4g.96gb+gfx" -> "4g.96gb.gfx"
func normalizeProfileName(profileStr string) string {
	return strings.ReplaceAll(profileStr, "+", ".")
}

// BuildMigConfigSpec creates a v1.Spec from discovered profiles
// This is the shared logic used by both nvidia-mig-parted generate-config
// and nvidia-mig-manager for consistent config generation.
func BuildMigConfigSpec(deviceProfiles discovery.DeviceProfiles) (*migspec.Spec, error) {
	// Group profiles by profile name across all devices
	// map[profileName]map[deviceID]discovery.ProfileInfo
	profileGroups := make(map[string]map[string]discovery.ProfileInfo)

	// Track all unique device IDs in the system
	allDeviceIDs := make(map[string]bool)

	for _, profiles := range deviceProfiles {
		for _, pInfo := range profiles {
			deviceIDStr := pInfo.DeviceID.String()
			allDeviceIDs[deviceIDStr] = true

			if profileGroups[pInfo.Name] == nil {
				profileGroups[pInfo.Name] = make(map[string]discovery.ProfileInfo)
			}
			profileGroups[pInfo.Name][deviceIDStr] = pInfo
		}
	}

	// Create config entries
	configs := map[string]migspec.MigConfigSpecSlice{}

	// Add base configs that should always be present
	configs["all-disabled"] = migspec.MigConfigSpecSlice{
		{
			Devices:    "all",
			MigEnabled: false,
		},
	}
	configs["all-enabled"] = migspec.MigConfigSpecSlice{
		{
			Devices:    "all",
			MigEnabled: true,
			MigDevices: types.MigConfig{},
		},
	}

	// Get sorted profile names for consistent output
	profileNames := make([]string, 0, len(profileGroups))
	for profileName := range profileGroups {
		profileNames = append(profileNames, profileName)
	}
	sort.Strings(profileNames)

	for _, profileName := range profileNames {
		devicesWithProfile := profileGroups[profileName]

		// Group devices by their max count for this profile
		// Some devices may support different max counts for the same profile
		countToDevices := make(map[int][]string)
		for deviceIDStr, pInfo := range devicesWithProfile {
			countToDevices[pInfo.MaxCount] = append(countToDevices[pInfo.MaxCount], deviceIDStr)
		}

		// Sort counts for consistent output
		counts := make([]int, 0, len(countToDevices))
		for count := range countToDevices {
			counts = append(counts, count)
		}
		sort.Ints(counts)

		// Normalize profile name for config (replace + with .)
		normalizedProfile := normalizeProfileName(profileName)
		configName := fmt.Sprintf("all-%s", normalizedProfile)
		configSpecs := make(migspec.MigConfigSpecSlice, 0)

		// Create a spec for each unique count
		for _, maxCount := range counts {
			deviceIDs := countToDevices[maxCount]
			sort.Strings(deviceIDs)

			// Create types.MigConfig for mig-devices field (use original profile name)
			migDevices := types.MigConfig{profileName: maxCount}

			// Build config spec
			spec := migspec.MigConfigSpec{
				Devices:    "all",
				MigEnabled: true,
				MigDevices: migDevices,
			}

			// Add device-filter if:
			// 1. Multiple unique device types exist in the system, OR
			// 2. Not all devices support this profile (partial support)
			if len(allDeviceIDs) > 1 || len(deviceIDs) < len(allDeviceIDs) {
				spec.DeviceFilter = deviceIDs
			}

			configSpecs = append(configSpecs, spec)

			log.Infof("Generated config '%s' for profile '%s' with max count %d (devices: %v)",
				configName, profileName, maxCount, deviceIDs)
		}

		configs[configName] = configSpecs
	}

	return &migspec.Spec{
		Version:    migspec.Version,
		MigConfigs: configs,
	}, nil
}

