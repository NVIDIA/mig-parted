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

package types

import (
	"fmt"
	"strconv"
	"strings"
)

// DeviceID represents a GPU Device ID as read from a GPUs PCIe config space.
type DeviceID struct {
	Device          uint16
	Vendor          uint16
	SubsystemDevice uint16
	SubsystemVendor uint16
	HasSubsystem    bool
}

// NewDeviceID constructs a new 'DeviceID' from the device and vendor values pulled from a GPUs PCIe config space.
func NewDeviceID(device, vendor uint16) DeviceID {
	return DeviceID{
		Device: device,
		Vendor: vendor,
	}
}

// NewDeviceIDWithSubsystem constructs a new 'DeviceID' with subsystem values.
func NewDeviceIDWithSubsystem(device, vendor, subDevice, subVendor uint16) DeviceID {
	return DeviceID{
		Device:          device,
		Vendor:          vendor,
		SubsystemDevice: subDevice,
		SubsystemVendor: subVendor,
		HasSubsystem:    true,
	}
}

// NewDeviceIDFromString constructs a 'DeviceID' from its string representation.
func NewDeviceIDFromString(str string) (DeviceID, error) {
	parts := strings.Split(str, ":")
	
	deviceIDRaw, err := strconv.ParseInt(parts[0], 0, 32)
	if err != nil {
		return DeviceID{}, fmt.Errorf("unable to create DeviceID from string '%v': %v", str, err)
	}

	deviceID := DeviceID{
		Device: uint16(deviceIDRaw >> 16),
		Vendor: uint16(deviceIDRaw),
	}

	if len(parts) == 2 {
		subIDRaw, err := strconv.ParseInt(parts[1], 0, 32)
		if err != nil {
			return DeviceID{}, fmt.Errorf("unable to create Subsystem from string '%v': %v", str, err)
		}
		deviceID.SubsystemDevice = uint16(subIDRaw >> 16)
		deviceID.SubsystemVendor = uint16(subIDRaw)
		deviceID.HasSubsystem = true
	}

	return deviceID, nil
}

// String returns a 'DeviceID' as a string.
func (d DeviceID) String() string {
	primary := fmt.Sprintf("0x%04X%04X", d.Device, d.Vendor)
	if d.HasSubsystem {
		return fmt.Sprintf("%s:0x%04X%04X", primary, d.SubsystemDevice, d.SubsystemVendor)
	}
	return primary
}

// GetVendor returns the 'vendor' portion of a 'DeviceID'.
func (d DeviceID) GetVendor() uint16 {
	return d.Vendor
}

// GetDevice returns the 'device' portion of a 'DeviceID'.
func (d DeviceID) GetDevice() uint16 {
	return d.Device
}

// Matches checks if a hardware GPU matches the DeviceID filter.
// If the filter has a subsystem defined, it requires an exact match on all 4 components.
// Otherwise, it only matches on the primary device and vendor IDs.
func (filter DeviceID) Matches(hardware DeviceID) bool {
	if filter.Device != hardware.Device || filter.Vendor != hardware.Vendor {
		return false
	}
	
	if filter.HasSubsystem {
		if filter.SubsystemDevice != hardware.SubsystemDevice || filter.SubsystemVendor != hardware.SubsystemVendor {
			return false
		}
	}
	
	return true
}
