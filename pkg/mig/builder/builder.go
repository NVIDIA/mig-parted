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
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"

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

// buildMigConfigSpec creates a v1.Spec from discovered profiles.
// This is an internal function - use GenerateConfigSpec() instead.
func buildMigConfigSpec(deviceProfiles discovery.DeviceProfiles) (*migspec.Spec, error) {
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

	// Build all-balanced config
	if balancedSpecs := buildAllBalancedConfig(deviceProfiles, allDeviceIDs); len(balancedSpecs) > 0 {
		configs["all-balanced"] = balancedSpecs
	}

	// Get sorted profile names for consistent output
	profileNames := slices.Sorted(maps.Keys(profileGroups))

	for _, profileName := range profileNames {
		devicesWithProfile := profileGroups[profileName]

		// Group devices by their max count for this profile
		// Some devices may support different max counts for the same profile
		countToDevices := make(map[int][]string)
		for deviceIDStr, pInfo := range devicesWithProfile {
			countToDevices[pInfo.MaxCount] = append(countToDevices[pInfo.MaxCount], deviceIDStr)
		}

		// Sort counts for consistent output
		counts := slices.Sorted(maps.Keys(countToDevices))

		normalizedProfile := normalizeProfileName(profileName)
		configName := fmt.Sprintf("all-%s", normalizedProfile)
		configSpecs := make(migspec.MigConfigSpecSlice, 0)

		// Create a spec for each unique count
		for _, maxCount := range counts {
			deviceIDs := countToDevices[maxCount]
			slices.Sort(deviceIDs)

			migDevices := types.MigConfig{profileName: maxCount}

			spec := migspec.MigConfigSpec{
				Devices:    "all",
				MigEnabled: true,
				MigDevices: migDevices,
			}

			// Add device-filter for heterogeneous GPU systems.
			// When multiple GPU types exist, each config entry specifies which devices
			// it applies to. This handles:
			// - Different GPU types with different profiles (e.g., A100 vs A30)
			// - Same profile name with different max counts across GPU types
			// - Same profile grouped together when max counts match
			//
			// Homogeneous systems (single GPU type) don't need device-filter since
			// all devices support the same profiles with the same max counts.
			if len(allDeviceIDs) > 1 {
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

// GenerateConfigSpec discovers MIG profiles from hardware and builds a config spec.
func GenerateConfigSpec() (*migspec.Spec, error) {
	deviceProfiles, err := discovery.DiscoverMIGProfiles()
	if err != nil {
		return nil, fmt.Errorf("failed to discover MIG profiles: %w", err)
	}
	return buildMigConfigSpec(deviceProfiles)
}

// GenerateConfigYAML discovers MIG profiles and generates the full config as YAML bytes.
func GenerateConfigYAML() ([]byte, error) {
	spec, err := GenerateConfigSpec()
	if err != nil {
		return nil, err
	}

	yamlData, err := yaml.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config to YAML: %w", err)
	}

	return yamlData, nil
}

// GenerateConfigJSON discovers MIG profiles and generates the full config as JSON bytes.
func GenerateConfigJSON() ([]byte, error) {
	spec, err := GenerateConfigSpec()
	if err != nil {
		return nil, err
	}

	jsonData, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config to JSON: %w", err)
	}

	return jsonData, nil
}
