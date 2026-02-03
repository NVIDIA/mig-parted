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

package discovery

import (
	"fmt"

	nvdev "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	log "github.com/sirupsen/logrus"

	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/util"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

const (
	// deviceIDA30 is the PCI device ID for A30-24GB GPUs.
	// NVML has a bug that reports incorrect profiles for this GPU.
	deviceIDA30 = 0x20B710DE
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

// discoverer holds dependencies for profile discovery, enabling testing with mocks
type discoverer struct {
	nvmllib   nvml.Interface
	deviceLib nvdev.Interface
}

// isCIProfile returns true if the profile is a Compute Instance profile.
// CI profiles have C > 0 and C < G (e.g. 1c.2g.20gb).
func isCIProfile(c, g int) bool {
	return c > 0 && c < g
}

// getDeviceID extracts the device ID from a nvdev.Device.
func getDeviceID(dev nvdev.Device) (types.DeviceID, error) {
	pciInfo, ret := nvml.Device(dev).GetPciInfo()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("failed to get PCI info: %v", ret)
	}
	return types.DeviceID(pciInfo.PciDeviceId), nil
}

// getHardcodedA30Profiles returns hardcoded MIG profiles for A30 GPUs.
// This is needed because NVML reports incorrect profiles for A30.
func getHardcodedA30Profiles(deviceID types.DeviceID) []ProfileInfo {
	return []ProfileInfo{
		{Name: "1g.6gb", MaxCount: 4, DeviceID: deviceID,
			Profile: nvdev.MigProfileInfo{C: 1, G: 1, GB: 6}},
		{Name: "1g.6gb+me", MaxCount: 1, DeviceID: deviceID,
			Profile: nvdev.MigProfileInfo{C: 1, G: 1, GB: 6, Attributes: []string{"me"}}},
		{Name: "2g.12gb", MaxCount: 2, DeviceID: deviceID,
			Profile: nvdev.MigProfileInfo{C: 2, G: 2, GB: 12}},
		{Name: "2g.12gb+me", MaxCount: 1, DeviceID: deviceID,
			Profile: nvdev.MigProfileInfo{C: 2, G: 2, GB: 12, Attributes: []string{"me"}}},
		{Name: "4g.24gb", MaxCount: 1, DeviceID: deviceID,
			Profile: nvdev.MigProfileInfo{C: 4, G: 4, GB: 24}},
	}
}

// DiscoverMIGProfiles discovers all MIG profiles on the system.
// Returns map[deviceIndex][]ProfileInfo for all MIG-capable devices.
func DiscoverMIGProfiles() (DeviceProfiles, error) {
	nvmllib := nvml.New()
	err := util.NvmlInit(nvmllib)
	if err != nil {
		return nil, fmt.Errorf("error initializing NVML: %w", err)
	}
	defer util.TryNvmlShutdown(nvmllib)

	deviceLib := nvdev.New(nvmllib)

	d := &discoverer{
		nvmllib:   nvmllib,
		deviceLib: deviceLib,
	}
	return d.discoverProfiles()
}

// discoverProfiles performs the actual discovery using injected dependencies.
func (d *discoverer) discoverProfiles() (DeviceProfiles, error) {
	result := make(DeviceProfiles)

	err := d.deviceLib.VisitDevices(func(i int, dev nvdev.Device) error {
		deviceID, err := getDeviceID(dev)
		if err != nil {
			return fmt.Errorf("error getting device ID for device %d: %w", i, err)
		}

		capable, err := dev.IsMigCapable()
		if err != nil {
			return fmt.Errorf("error checking if device %d is MIG-capable: %w", i, err)
		}

		if !capable {
			log.Infof("Device %d (DeviceID: %s) is not MIG-capable, skipping", i, deviceID.String())
			return nil
		}

		// Check for A30 - use hardcoded profiles due to NVML bug where
		// GetGpuInstanceProfileInfo returns incorrect InstanceCount values.
		// The hardcoded values match the A30 MIG profiles from nvidia-smi.
		if uint32(deviceID) == deviceIDA30 {
			log.Infof("Device %d is A30 (DeviceID: %s), using hardcoded profiles due to NVML bug",
				i, deviceID.String())
			result[i] = getHardcodedA30Profiles(deviceID)
			return nil
		}

		log.Infof("Discovering MIG profiles for device %d (DeviceID: %s)", i, deviceID.String())

		profiles, err := dev.GetMigProfiles()
		if err != nil {
			return fmt.Errorf("error getting MIG profiles for device %d: %w", i, err)
		}

		if len(profiles) == 0 {
			log.Warnf("No MIG profiles found for device %d", i)
			return nil
		}

		log.Infof("Found %d MIG profile(s) for device %d", len(profiles), i)

		nvmlDevice := nvml.Device(dev)
		var deviceProfiles []ProfileInfo

		for _, profile := range profiles {
			profileInfo := profile.GetInfo()
			profileStr := profile.String()

			// Skip Compute Instance (CI) profiles - these are sub-partitions of GPU Instances and not used for
			// "all-*" MIG configurations. We do not have a use case currently where 'all-*' MIG profiles should
			// provision MIG slices that leave some SMs/compute slices unutilized.
			// CI profiles have C > 0 and C != G.
			// Example: 1c.3g.20gb is a CI profile (1 compute slice in a 3g GPU instance), while 3g.20gb is a GI
			// profile (full 3g GPU instance).
			if isCIProfile(profileInfo.C, profileInfo.G) {
				log.Infof("Skipping Compute Instance profile %s (C=%d, G=%d)", profileStr, profileInfo.C, profileInfo.G)
				continue
			}

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
				DeviceID: deviceID,
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
