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

package generateconfig

import (
	"fmt"
	"sort"
	"strings"

	nvdev "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	v1 "github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/util"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

// DeviceProfileInfo holds profile information for a specific device
type DeviceProfileInfo struct {
	DeviceID types.DeviceID
	Index    int
	Profiles []nvdev.MigProfile
}

// ProfileWithCount holds a MIG profile and its maximum instance count
type ProfileWithCount struct {
	Profile  nvdev.MigProfile
	MaxCount int
}

// normalizeProfileName converts profile names to config-friendly format
// Replaces '+' with '.' for attributes like +me, +gfx, +me.all
// Examples: "1g.24gb+me" -> "1g.24gb.me", "4g.96gb+gfx" -> "4g.96gb.gfx"
func normalizeProfileName(profileStr string) string {
	return strings.ReplaceAll(profileStr, "+", ".")
}

// GenerateMigConfigSpec discovers MIG profiles on all GPUs and generates a config spec
func GenerateMigConfigSpec(c *Context) (*v1.Spec, error) {
	// Step 1: Get device IDs using existing utility
	deviceIDs, err := util.GetGPUDeviceIDs()
	if err != nil {
		return nil, fmt.Errorf("error enumerating GPUs: %v", err)
	}

	if len(deviceIDs) == 0 {
		return nil, fmt.Errorf("no GPUs found on the system")
	}

	log.Infof("Found %d GPU(s) on the system", len(deviceIDs))

	// Step 2: Initialize NVML using existing utility
	nvmllib := nvml.New()
	err = util.NvmlInit(nvmllib)
	if err != nil {
		return nil, fmt.Errorf("error initializing NVML: %v", err)
	}
	defer util.TryNvmlShutdown(nvmllib)

	// Step 3: Create go-nvlib device interface
	deviceLib := nvdev.New(nvmllib)

	// Step 4: Collect profiles with their max counts from each device
	profilesByDevice := make(map[int]map[string]ProfileWithCount)

	err = deviceLib.VisitDevices(func(i int, d nvdev.Device) error {
		// Check if device is MIG-capable
		capable, err := d.IsMigCapable()
		if err != nil {
			return fmt.Errorf("error checking if device %d is MIG-capable: %v", i, err)
		}

		if !capable {
			log.Infof("Device %d (DeviceID: %s) is not MIG-capable, skipping", i, deviceIDs[i].String())
			return nil
		}

		log.Infof("Discovering MIG profiles for device %d (DeviceID: %s)", i, deviceIDs[i].String())

		// Use go-nvlib's GetMigProfiles (works without MIG enabled)
		profiles, err := d.GetMigProfiles()
		if err != nil {
			return fmt.Errorf("error getting MIG profiles for device %d: %v", i, err)
		}

		if len(profiles) == 0 {
			log.Warnf("No MIG profiles found for device %d", i)
			return nil
		}

		log.Infof("Found %d MIG profile(s) for device %d", len(profiles), i)

		// Get the underlying NVML device to query GPU instance profile info
		nvmlDevice := nvml.Device(d)

		// Map to store profiles with their max counts for this device
		deviceProfiles := make(map[string]ProfileWithCount)

		// For each profile, get its max instance count from GPU instance profile info
		for _, profile := range profiles {
			profileInfo := profile.GetInfo()
			profileStr := profile.String()

			// Skip Compute Instance (CI) profiles - we only want GPU Instance (GI) profiles
			// CI profiles have the format like "1c.2g.20gb" where 'c' indicates compute instance
			// GI profiles have the format like "1g.5gb", "2g.10gb", "7g.80gb"
			// The profileInfo.C field represents the compute slice count
			// If C > 0 and C < G, it's a CI profile (subdivided GPU instance)
			if profileInfo.C > 0 && profileInfo.C < profileInfo.G {
				log.Infof("Skipping Compute Instance profile %s (C=%d, G=%d)", profileStr, profileInfo.C, profileInfo.G)
				continue
			}

			// Query the GPU instance profile info to get the max instance count
			giProfileInfo, ret := nvmlDevice.GetGpuInstanceProfileInfo(profileInfo.GIProfileID)
			if ret != nvml.SUCCESS {
				log.Warnf("Could not get GPU instance profile info for profile %s (GI ID: %d): %v",
					profileStr, profileInfo.GIProfileID, ret)
				continue
			}

			maxCount := int(giProfileInfo.InstanceCount)

			// Store the profile with its max count
			// If we see the same profile string multiple times (shouldn't happen with GetMigProfiles),
			// keep the one with the highest count
			if existing, found := deviceProfiles[profileStr]; !found || maxCount > existing.MaxCount {
				deviceProfiles[profileStr] = ProfileWithCount{
					Profile:  profile,
					MaxCount: maxCount,
				}
			}

			log.Debugf("Profile %s on device %d has max count: %d", profileStr, i, maxCount)
		}

		if len(deviceProfiles) == 0 {
			log.Warnf("No valid MIG profiles with instance counts found for device %d", i)
			return nil
		}

		profilesByDevice[i] = deviceProfiles

		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(profilesByDevice) == 0 {
		return nil, fmt.Errorf("no MIG-capable devices found on the system")
	}

	// Step 5: Build the config specification
	return buildMigConfigSpec(profilesByDevice, deviceIDs, c.Flags.ConfigLabel)
}

// buildMigConfigSpec creates a v1.Spec from discovered profiles
func buildMigConfigSpec(profilesByDevice map[int]map[string]ProfileWithCount, deviceIDs []types.DeviceID, configLabel string) (*v1.Spec, error) {
	// Group profiles by profile string across all devices
	// map[profileString]map[deviceID]ProfileWithCount
	profileGroups := make(map[string]map[string]ProfileWithCount)

	for deviceIdx, profiles := range profilesByDevice {
		deviceID := deviceIDs[deviceIdx]
		deviceIDStr := deviceID.String()

		for profileStr, pwc := range profiles {
			if profileGroups[profileStr] == nil {
				profileGroups[profileStr] = make(map[string]ProfileWithCount)
			}
			profileGroups[profileStr][deviceIDStr] = pwc
		}
	}

	// Get all unique device IDs in the system
	allDeviceIDs := make(map[string]bool)
	for deviceIdx := range profilesByDevice {
		allDeviceIDs[deviceIDs[deviceIdx].String()] = true
	}

	// Create config entries
	configs := map[string]v1.MigConfigSpecSlice{}

	// Add base configs that should always be present
	configs["all-disabled"] = v1.MigConfigSpecSlice{
		{
			Devices:    "all",
			MigEnabled: false,
		},
	}
	configs["all-enabled"] = v1.MigConfigSpecSlice{
		{
			Devices:    "all",
			MigEnabled: true,
			MigDevices: types.MigConfig{},
		},
	}

	// Get sorted profile names for consistent output
	profileNames := make([]string, 0, len(profileGroups))
	for profileStr := range profileGroups {
		profileNames = append(profileNames, profileStr)
	}
	sort.Strings(profileNames)

	for _, profileStr := range profileNames {
		devicesWithProfile := profileGroups[profileStr]

		// Group devices by their max count for this profile
		// Some devices may support different max counts for the same profile
		countToDevices := make(map[int][]string)
		for deviceIDStr, pwc := range devicesWithProfile {
			countToDevices[pwc.MaxCount] = append(countToDevices[pwc.MaxCount], deviceIDStr)
		}

		// Sort counts for consistent output
		counts := make([]int, 0, len(countToDevices))
		for count := range countToDevices {
			counts = append(counts, count)
		}
		sort.Ints(counts)

		// Normalize profile name for config (replace + with .)
		normalizedProfile := normalizeProfileName(profileStr)
		configName := fmt.Sprintf("all-%s", normalizedProfile)
		configSpecs := make(v1.MigConfigSpecSlice, 0)

		// Create a spec for each unique count
		for _, maxCount := range counts {
			deviceIDs := countToDevices[maxCount]
			sort.Strings(deviceIDs)

			// Create types.MigConfig for mig-devices field
			migDevices := types.MigConfig{profileStr: maxCount}

			// Build config spec
			spec := v1.MigConfigSpec{
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
				configName, profileStr, maxCount, deviceIDs)
		}

		configs[configName] = configSpecs
	}

	return &v1.Spec{
		Version:    v1.Version,
		MigConfigs: configs,
	}, nil
}
