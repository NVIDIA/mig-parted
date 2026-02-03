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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/mig-parted/pkg/mig/discovery"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

// expectedBalanced defines the all-balanced config for each GPU type.
// Formula: 7-slot: 2×1g + 1×2g + 1×3g, 4-slot: 2×1g + 1×2g
var expectedBalanced = map[string]types.MigConfig{
	"A30-24GB":     {"1g.6gb": 2, "2g.12gb": 1},
	"A100-40GB":    {"1g.5gb": 2, "2g.10gb": 1, "3g.20gb": 1},
	"A100-80GB":    {"1g.10gb": 2, "2g.20gb": 1, "3g.40gb": 1},
	"H100-80GB":    {"1g.10gb": 2, "2g.20gb": 1, "3g.40gb": 1},
	"H100-94GB":    {"1g.12gb": 2, "2g.24gb": 1, "3g.47gb": 1},
	"H100-96GB":    {"1g.12gb": 2, "2g.24gb": 1, "3g.48gb": 1},
	"H200-141GB":   {"1g.18gb": 2, "2g.35gb": 1, "3g.71gb": 1},
	"B200-180GB":   {"1g.23gb": 2, "2g.45gb": 1, "3g.90gb": 1},
	"RTX-PRO-6000": {"1g.24gb": 2, "2g.48gb": 1},
	// Note: RTX-PRO-5000 cannot generate all-balanced (no base 2g profile)
}

func TestBuildAllBalancedConfig(t *testing.T) {
	testCases := []struct {
		name           string
		deviceProfiles discovery.DeviceProfiles
		wantNumConfigs int
		wantMigDevices []types.MigConfig
		wantDeviceIDs  []string
	}{
		{
			name:           "7-slot GPU",
			deviceProfiles: discovery.DeviceProfiles{0: gpuProfiles["A100-80GB"]},
			wantNumConfigs: 1,
			wantMigDevices: []types.MigConfig{expectedBalanced["A100-80GB"]},
		},
		{
			name:           "4-slot GPU",
			deviceProfiles: discovery.DeviceProfiles{0: gpuProfiles["A30-24GB"]},
			wantNumConfigs: 1,
			wantMigDevices: []types.MigConfig{expectedBalanced["A30-24GB"]},
		},
		{
			name:           "heterogeneous GPUs",
			deviceProfiles: discovery.DeviceProfiles{0: gpuProfiles["A100-80GB"], 1: gpuProfiles["A30-24GB"]},
			wantNumConfigs: 2,
			wantMigDevices: []types.MigConfig{expectedBalanced["A100-80GB"], expectedBalanced["A30-24GB"]},
			wantDeviceIDs:  []string{idA100_80GB, idA30},
		},
		{
			name: "only attributed profiles - no balanced config",
			deviceProfiles: discovery.DeviceProfiles{
				0: {
					{Name: "1g.10gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(1, 10, []string{"me"}, nil)},
					{Name: "2g.20gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(2, 20, []string{"me"}, nil)},
				},
			},
			wantNumConfigs: 0,
			wantMigDevices: nil,
		},
		{
			name: "missing 2g profile - no balanced config",
			deviceProfiles: discovery.DeviceProfiles{
				0: {
					{Name: "1g.10gb", MaxCount: 7, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(1, 10, nil, nil)},
					{Name: "3g.40gb", MaxCount: 2, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(3, 40, nil, nil)},
				},
			},
			wantNumConfigs: 0,
			wantMigDevices: nil,
		},
		{
			name: "2-slot GPU - no balanced config",
			deviceProfiles: discovery.DeviceProfiles{
				0: {
					{Name: "1g.16gb", MaxCount: 2, DeviceID: mockDeviceID(0x2BB610DE), Profile: mockProfile(1, 16, nil, nil)},
					{Name: "2g.32gb", MaxCount: 1, DeviceID: mockDeviceID(0x2BB610DE), Profile: mockProfile(2, 32, nil, nil)},
				},
			},
			wantNumConfigs: 0,
			wantMigDevices: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Build allDeviceIDs from deviceProfiles
			allDeviceIDs := make(map[string]bool)
			for _, profiles := range tc.deviceProfiles {
				for _, p := range profiles {
					allDeviceIDs[p.DeviceID.String()] = true
				}
			}

			result := buildAllBalancedConfig(tc.deviceProfiles, allDeviceIDs)

			require.Len(t, result, tc.wantNumConfigs, "unexpected number of config specs")

			if tc.wantNumConfigs == 0 {
				return
			}

			// Verify each expected mig-devices config exists in results
			for _, wantMigDevices := range tc.wantMigDevices {
				found := false
				for _, spec := range result {
					if spec.MigDevices.Equals(wantMigDevices) {
						found = true
						assert.Equal(t, "all", spec.Devices)
						assert.True(t, spec.MigEnabled)
						break
					}
				}
				assert.True(t, found, "expected mig-devices %v not found in result", wantMigDevices)
			}

			// Verify device-filter is set for heterogeneous (multiple device types)
			if len(allDeviceIDs) > 1 {
				// Collect all device IDs from filters
				foundDeviceIDs := make(map[string]bool)
				for _, spec := range result {
					filter, ok := spec.DeviceFilter.([]string)
					require.True(t, ok, "device-filter should be set for heterogeneous devices")
					require.Len(t, filter, 1, "device-filter should have one device ID")
					foundDeviceIDs[filter[0]] = true
				}

				// Verify expected device IDs are present
				for _, wantID := range tc.wantDeviceIDs {
					assert.True(t, foundDeviceIDs[wantID], "expected device ID %s not found in filters", wantID)
				}
			} else {
				// Single device type - no device-filter
				for _, spec := range result {
					assert.Nil(t, spec.DeviceFilter, "device-filter should not be set for single device type")
				}
			}
		})
	}
}
