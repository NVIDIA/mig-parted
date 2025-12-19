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

package discovery

import (
	"fmt"

	nvdev "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	log "github.com/sirupsen/logrus"

	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/util"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

// ProfileInfo represents a discovered MIG profile with its metadata
type ProfileInfo struct {
	Name     string           // Profile name (e.g., "1g.10gb", "2g.20gb")
	MaxCount int              // Maximum instance count for this profile
	DeviceID types.DeviceID   // Device ID where this profile was discovered
	Profile  nvdev.MigProfile // The underlying MIG profile object
}

// DeviceProfiles maps device index to its discovered profiles
type DeviceProfiles map[int][]ProfileInfo

// DiscoverMIGProfiles discovers all MIG profiles on the system
// Returns map[deviceIndex][]ProfileInfo for all MIG-capable devices
func DiscoverMIGProfiles() (DeviceProfiles, error) {
	// Step 1: Get device IDs using existing utility
	// This method also checks if the NVIDIA module is loaded
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

	// Step 4: Collect profiles from each device
	result := make(DeviceProfiles)

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

		// Collect profiles for this device
		deviceProfiles := []ProfileInfo{}

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

			deviceProfiles = append(deviceProfiles, ProfileInfo{
				Name:     profileStr,
				MaxCount: maxCount,
				DeviceID: deviceIDs[i],
				Profile:  profile,
			})

			log.Debugf("Profile %s on device %d has max count: %d", profileStr, i, maxCount)
		}

		if len(deviceProfiles) == 0 {
			log.Warnf("No valid MIG profiles with instance counts found for device %d", i)
			return nil
		}

		result[i] = deviceProfiles

		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no MIG-capable devices found on the system")
	}

	return result, nil
}
