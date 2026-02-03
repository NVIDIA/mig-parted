/*
 * Copyright (c) NVIDIA CORPORATION.  All rights reserved.
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
	"maps"
	"slices"

	log "github.com/sirupsen/logrus"

	migspec "github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/pkg/mig/discovery"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

// balancedFormula defines the instance counts for each slot configuration.
// Key is max slots (7 or 4), value is map of G-value to instance count.
var balancedFormula = map[int]map[int]int{
	7: {1: 2, 2: 1, 3: 1}, // 2×1g + 1×2g + 1×3g = 7 slices
	4: {1: 2, 2: 1},       // 2×1g + 1×2g = 4 slices
}

// isBaseProfile returns true if the profile has no special attributes (+me, +gfx, etc.)
// Base profiles are used for all-balanced config since they're the "standard" variants.
func isBaseProfile(pInfo discovery.ProfileInfo) bool {
	info := pInfo.Profile.GetInfo()
	return len(info.Attributes) == 0 && len(info.NegAttributes) == 0
}

// deviceProfileSet groups profiles by their G value (slice count) for a single device type.
// Used to find 1g, 2g, 3g profiles for all-balanced config.
type deviceProfileSet struct {
	deviceID string
	// profiles maps G value to the base profile for that slice count
	profiles map[int]discovery.ProfileInfo
	// totalSlots is the total GPU slot count derived from the profile's G value (e.g., 2g=2, 4g=4, 7g=7)
	totalSlots int
}

// canBuildBalanced checks if we have all required profiles for the balanced config.
func (dps *deviceProfileSet) canBuildBalanced() bool {
	formula, ok := balancedFormula[dps.totalSlots]
	if !ok {
		return false
	}
	for gValue := range formula {
		if _, hasProfile := dps.profiles[gValue]; !hasProfile {
			return false
		}
	}
	return true
}

// buildMigDevices creates the mig-devices map for the balanced config.
func (dps *deviceProfileSet) buildMigDevices() types.MigConfig {
	formula := balancedFormula[dps.totalSlots]

	migDevices := make(types.MigConfig)
	for gValue, count := range formula {
		profile := dps.profiles[gValue]
		migDevices[profile.Name] = count
	}
	return migDevices
}

// groupProfilesByDevice organizes profiles by device ID and G value,
// keeping only base profiles (no +me/+gfx attributes).
func groupProfilesByDevice(deviceProfiles discovery.DeviceProfiles) map[string]*deviceProfileSet {
	result := make(map[string]*deviceProfileSet)

	for _, profiles := range deviceProfiles {
		for _, pInfo := range profiles {
			// Skip non-base profiles (those with +me, +gfx, etc.)
			if !isBaseProfile(pInfo) {
				continue
			}

			deviceID := pInfo.DeviceID.String()
			gValue := pInfo.Profile.GetInfo().G

			if result[deviceID] == nil {
				result[deviceID] = &deviceProfileSet{
					deviceID: deviceID,
					profiles: make(map[int]discovery.ProfileInfo),
				}
			}

			// Track max G value to determine total slots.
			if gValue > result[deviceID].totalSlots {
				result[deviceID].totalSlots = gValue
			}

			// Only store 1g, 2g, 3g profiles for balanced formula
			if gValue < 1 || gValue > 3 {
				continue
			}

			// Keep the profile with the highest MaxCount for each G value.
			// This gives us the smallest memory footprint (more flexible) option.
			// e.g., prefer 1g.10gb (max 7) over 1g.20gb (max 4)
			existing, exists := result[deviceID].profiles[gValue]
			if !exists || pInfo.MaxCount > existing.MaxCount {
				result[deviceID].profiles[gValue] = pInfo
			}
		}
	}

	return result
}

// buildAllBalancedConfig creates the "all-balanced" config entry.
// Returns nil if no devices can support a balanced config.
func buildAllBalancedConfig(deviceProfiles discovery.DeviceProfiles, allDeviceIDs map[string]bool) migspec.MigConfigSpecSlice {
	deviceSets := groupProfilesByDevice(deviceProfiles)

	var configSpecs migspec.MigConfigSpecSlice

	// Get sorted device IDs for consistent output
	sortedDeviceIDs := slices.Sorted(maps.Keys(deviceSets))

	for _, deviceID := range sortedDeviceIDs {
		dps := deviceSets[deviceID]

		if !dps.canBuildBalanced() {
			log.Warnf("Device %s missing required profiles for all-balanced config, skipping", deviceID)
			continue
		}

		spec := migspec.MigConfigSpec{
			Devices:    "all",
			MigEnabled: true,
			MigDevices: dps.buildMigDevices(),
		}

		// Add device-filter if multiple device types exist
		if len(allDeviceIDs) > 1 {
			spec.DeviceFilter = []string{deviceID}
		}

		configSpecs = append(configSpecs, spec)

		log.Infof("Generated all-balanced config for device %s (max slots: %d)", deviceID, dps.totalSlots)
	}

	return configSpecs
}
