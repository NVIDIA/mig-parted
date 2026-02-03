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
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nvdev "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"

	v1 "github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/pkg/mig/discovery"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

func mockProfile(g, gb int, attrs, negAttrs []string) nvdev.MigProfile {
	return nvdev.MigProfileInfo{
		C:             g,
		G:             g,
		GB:            gb,
		Attributes:    attrs,
		NegAttributes: negAttrs,
	}
}

func mockDeviceID(id uint32) types.DeviceID {
	device := uint16(id >> 16)
	vendor := uint16(id & 0xFFFF)
	return types.NewDeviceID(device, vendor)
}

// elementsMatch checks if two string slices contain the same elements (order-independent)
func elementsMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := slices.Clone(a)
	bCopy := slices.Clone(b)
	slices.Sort(aCopy)
	slices.Sort(bCopy)
	return slices.Equal(aCopy, bCopy)
}

const (
	deviceIDA30       = 0x20B710DE // A30-24GB
	deviceIDA100_40GB = 0x20B010DE // A100-SXM4-40GB
	deviceIDA100_80GB = 0x20B510DE // A100-SXM4-80GB
	deviceIDH100_80GB = 0x233110DE // H100-80GB
	deviceIDH100_94GB = 0x233210DE // H100-94GB (placeholder ID)
	deviceIDH100_96GB = 0x233510DE // H100-96GB on GH200 (placeholder ID)
	deviceIDH200      = 0x233310DE // H200-141GB (placeholder ID)
	deviceIDB200      = 0x233410DE // B200-180GB (placeholder ID)
	deviceIDRTXPRO5   = 0x2BB410DE // RTX PRO 5000 Blackwell (placeholder ID)
	deviceIDRTXPRO6   = 0x2BB510DE // RTX PRO 6000 Blackwell
)

// String versions of device IDs for use in device-filter assertions
var (
	idA30       = mockDeviceID(deviceIDA30).String()
	idA100_80GB = mockDeviceID(deviceIDA100_80GB).String()
	idH100_80GB = mockDeviceID(deviceIDH100_80GB).String()
	idOther     = mockDeviceID(0x20B210DE).String() // hypothetical second GPU
)

// Reference: https://docs.nvidia.com/datacenter/tesla/mig-user-guide/supported-mig-profiles.html
var gpuProfiles = map[string][]discovery.ProfileInfo{
	"A30-24GB": {
		{Name: "1g.6gb", MaxCount: 4, DeviceID: mockDeviceID(deviceIDA30), Profile: mockProfile(1, 6, nil, nil)},
		{Name: "1g.6gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA30), Profile: mockProfile(1, 6, []string{"me"}, nil)},
		{Name: "2g.12gb", MaxCount: 2, DeviceID: mockDeviceID(deviceIDA30), Profile: mockProfile(2, 12, nil, nil)},
		{Name: "2g.12gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA30), Profile: mockProfile(2, 12, []string{"me"}, nil)},
		{Name: "4g.24gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA30), Profile: mockProfile(4, 24, nil, nil)},
	},
	"A100-40GB": {
		{Name: "1g.5gb", MaxCount: 7, DeviceID: mockDeviceID(deviceIDA100_40GB), Profile: mockProfile(1, 5, nil, nil)},
		{Name: "1g.5gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA100_40GB), Profile: mockProfile(1, 5, []string{"me"}, nil)},
		{Name: "1g.10gb", MaxCount: 4, DeviceID: mockDeviceID(deviceIDA100_40GB), Profile: mockProfile(1, 10, nil, nil)},
		{Name: "2g.10gb", MaxCount: 3, DeviceID: mockDeviceID(deviceIDA100_40GB), Profile: mockProfile(2, 10, nil, nil)},
		{Name: "3g.20gb", MaxCount: 2, DeviceID: mockDeviceID(deviceIDA100_40GB), Profile: mockProfile(3, 20, nil, nil)},
		{Name: "4g.20gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA100_40GB), Profile: mockProfile(4, 20, nil, nil)},
		{Name: "7g.40gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA100_40GB), Profile: mockProfile(7, 40, nil, nil)},
	},
	"A100-80GB": {
		{Name: "1g.10gb", MaxCount: 7, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(1, 10, nil, nil)},
		{Name: "1g.10gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(1, 10, []string{"me"}, nil)},
		{Name: "1g.20gb", MaxCount: 4, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(1, 20, nil, nil)},
		{Name: "2g.20gb", MaxCount: 3, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(2, 20, nil, nil)},
		{Name: "3g.40gb", MaxCount: 2, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(3, 40, nil, nil)},
		{Name: "4g.40gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(4, 40, nil, nil)},
		{Name: "7g.80gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(7, 80, nil, nil)},
	},
	"H100-80GB": {
		{Name: "1g.10gb", MaxCount: 7, DeviceID: mockDeviceID(deviceIDH100_80GB), Profile: mockProfile(1, 10, nil, nil)},
		{Name: "1g.10gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH100_80GB), Profile: mockProfile(1, 10, []string{"me"}, nil)},
		{Name: "1g.20gb", MaxCount: 4, DeviceID: mockDeviceID(deviceIDH100_80GB), Profile: mockProfile(1, 20, nil, nil)},
		{Name: "2g.20gb", MaxCount: 3, DeviceID: mockDeviceID(deviceIDH100_80GB), Profile: mockProfile(2, 20, nil, nil)},
		{Name: "3g.40gb", MaxCount: 2, DeviceID: mockDeviceID(deviceIDH100_80GB), Profile: mockProfile(3, 40, nil, nil)},
		{Name: "4g.40gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH100_80GB), Profile: mockProfile(4, 40, nil, nil)},
		{Name: "7g.80gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH100_80GB), Profile: mockProfile(7, 80, nil, nil)},
	},
	"H100-94GB": {
		{Name: "1g.12gb", MaxCount: 7, DeviceID: mockDeviceID(deviceIDH100_94GB), Profile: mockProfile(1, 12, nil, nil)},
		{Name: "1g.12gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH100_94GB), Profile: mockProfile(1, 12, []string{"me"}, nil)},
		{Name: "1g.24gb", MaxCount: 4, DeviceID: mockDeviceID(deviceIDH100_94GB), Profile: mockProfile(1, 24, nil, nil)},
		{Name: "2g.24gb", MaxCount: 3, DeviceID: mockDeviceID(deviceIDH100_94GB), Profile: mockProfile(2, 24, nil, nil)},
		{Name: "3g.47gb", MaxCount: 2, DeviceID: mockDeviceID(deviceIDH100_94GB), Profile: mockProfile(3, 47, nil, nil)},
		{Name: "4g.47gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH100_94GB), Profile: mockProfile(4, 47, nil, nil)},
		{Name: "7g.94gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH100_94GB), Profile: mockProfile(7, 94, nil, nil)},
	},
	"H100-96GB": {
		{Name: "1g.12gb", MaxCount: 7, DeviceID: mockDeviceID(deviceIDH100_96GB), Profile: mockProfile(1, 12, nil, nil)},
		{Name: "1g.12gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH100_96GB), Profile: mockProfile(1, 12, []string{"me"}, nil)},
		{Name: "1g.24gb", MaxCount: 4, DeviceID: mockDeviceID(deviceIDH100_96GB), Profile: mockProfile(1, 24, nil, nil)},
		{Name: "2g.24gb", MaxCount: 3, DeviceID: mockDeviceID(deviceIDH100_96GB), Profile: mockProfile(2, 24, nil, nil)},
		{Name: "3g.48gb", MaxCount: 2, DeviceID: mockDeviceID(deviceIDH100_96GB), Profile: mockProfile(3, 48, nil, nil)},
		{Name: "4g.48gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH100_96GB), Profile: mockProfile(4, 48, nil, nil)},
		{Name: "7g.96gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH100_96GB), Profile: mockProfile(7, 96, nil, nil)},
	},
	"H200-141GB": {
		{Name: "1g.18gb", MaxCount: 7, DeviceID: mockDeviceID(deviceIDH200), Profile: mockProfile(1, 18, nil, nil)},
		{Name: "1g.18gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH200), Profile: mockProfile(1, 18, []string{"me"}, nil)},
		{Name: "1g.35gb", MaxCount: 4, DeviceID: mockDeviceID(deviceIDH200), Profile: mockProfile(1, 35, nil, nil)},
		{Name: "2g.35gb", MaxCount: 3, DeviceID: mockDeviceID(deviceIDH200), Profile: mockProfile(2, 35, nil, nil)},
		{Name: "3g.71gb", MaxCount: 2, DeviceID: mockDeviceID(deviceIDH200), Profile: mockProfile(3, 71, nil, nil)},
		{Name: "4g.71gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH200), Profile: mockProfile(4, 71, nil, nil)},
		{Name: "7g.141gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDH200), Profile: mockProfile(7, 141, nil, nil)},
	},
	"B200-180GB": {
		{Name: "1g.23gb", MaxCount: 7, DeviceID: mockDeviceID(deviceIDB200), Profile: mockProfile(1, 23, nil, nil)},
		{Name: "1g.23gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDB200), Profile: mockProfile(1, 23, []string{"me"}, nil)},
		{Name: "1g.45gb", MaxCount: 4, DeviceID: mockDeviceID(deviceIDB200), Profile: mockProfile(1, 45, nil, nil)},
		{Name: "2g.45gb", MaxCount: 3, DeviceID: mockDeviceID(deviceIDB200), Profile: mockProfile(2, 45, nil, nil)},
		{Name: "3g.90gb", MaxCount: 2, DeviceID: mockDeviceID(deviceIDB200), Profile: mockProfile(3, 90, nil, nil)},
		{Name: "4g.90gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDB200), Profile: mockProfile(4, 90, nil, nil)},
		{Name: "7g.180gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDB200), Profile: mockProfile(7, 180, nil, nil)},
	},
	"RTX-PRO-5000": {
		{Name: "1g.12gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO5), Profile: mockProfile(1, 12, nil, nil)},
		{Name: "1g.12gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO5), Profile: mockProfile(1, 12, []string{"me"}, nil)},
		{Name: "1g.12gb+gfx", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO5), Profile: mockProfile(1, 12, []string{"gfx"}, nil)},
		{Name: "1g.12gb-me", MaxCount: 3, DeviceID: mockDeviceID(deviceIDRTXPRO5), Profile: mockProfile(1, 12, nil, []string{"me"})},
		{Name: "2g.24gb-me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO5), Profile: mockProfile(2, 24, nil, []string{"me"})},
		{Name: "4g.48gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO5), Profile: mockProfile(4, 48, nil, nil)},
		{Name: "4g.48gb+gfx", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO5), Profile: mockProfile(4, 48, []string{"gfx"}, nil)},
	},
	"RTX-PRO-6000": {
		{Name: "1g.24gb", MaxCount: 4, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(1, 24, nil, nil)},
		{Name: "1g.24gb+me", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(1, 24, []string{"me"}, nil)},
		{Name: "1g.24gb+gfx", MaxCount: 4, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(1, 24, []string{"gfx"}, nil)},
		{Name: "1g.24gb+me.all", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(1, 24, []string{"me.all"}, nil)},
		{Name: "1g.24gb-me", MaxCount: 4, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(1, 24, nil, []string{"me"})},
		{Name: "2g.48gb", MaxCount: 2, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(2, 48, nil, nil)},
		{Name: "2g.48gb+gfx", MaxCount: 2, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(2, 48, []string{"gfx"}, nil)},
		{Name: "2g.48gb+me.all", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(2, 48, []string{"me.all"}, nil)},
		{Name: "2g.48gb-me", MaxCount: 2, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(2, 48, nil, []string{"me"})},
		{Name: "4g.96gb", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(4, 96, nil, nil)},
		{Name: "4g.96gb+gfx", MaxCount: 1, DeviceID: mockDeviceID(deviceIDRTXPRO6), Profile: mockProfile(4, 96, []string{"gfx"}, nil)},
	},
}

func TestNormalizeProfileName(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"1g.10gb", "1g.10gb"},
		{"1g.10gb+me", "1g.10gb.me"},
		{"4g.96gb+gfx", "4g.96gb.gfx"},
		{"1g.24gb+me.all", "1g.24gb.me.all"},
		{"1g.24gb-me", "1g.24gb-me"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeProfileName(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

type wantConfig struct {
	configName   string // e.g., "all-1g.10gb"
	profileName  string // e.g., "1g.10gb"
	count        int
	deviceFilter []string // expected device IDs in filter, nil = no filter
}

func TestBuildMigConfigSpec(t *testing.T) {
	testCases := []struct {
		name           string
		deviceProfiles discovery.DeviceProfiles
		wantConfigs    []wantConfig
	}{
		{
			name:           "A100-80GB",
			deviceProfiles: discovery.DeviceProfiles{0: gpuProfiles["A100-80GB"]},
			wantConfigs: []wantConfig{
				// Balanced
				{"all-balanced", "1g.10gb", 2, nil},
				{"all-balanced", "2g.20gb", 1, nil},
				{"all-balanced", "3g.40gb", 1, nil},
				// Per-profile
				{"all-1g.10gb", "1g.10gb", 7, nil},
				{"all-1g.10gb.me", "1g.10gb+me", 1, nil},
				{"all-1g.20gb", "1g.20gb", 4, nil},
				{"all-2g.20gb", "2g.20gb", 3, nil},
				{"all-3g.40gb", "3g.40gb", 2, nil},
				{"all-4g.40gb", "4g.40gb", 1, nil},
				{"all-7g.80gb", "7g.80gb", 1, nil},
			},
		},
		{
			name:           "A30-24GB (4-slot)",
			deviceProfiles: discovery.DeviceProfiles{0: gpuProfiles["A30-24GB"]},
			wantConfigs: []wantConfig{
				// Balanced
				{"all-balanced", "1g.6gb", 2, nil},
				{"all-balanced", "2g.12gb", 1, nil},
				// Per-profile
				{"all-1g.6gb", "1g.6gb", 4, nil},
				{"all-1g.6gb.me", "1g.6gb+me", 1, nil},
				{"all-2g.12gb", "2g.12gb", 2, nil},
				{"all-2g.12gb.me", "2g.12gb+me", 1, nil},
				{"all-4g.24gb", "4g.24gb", 1, nil},
			},
		},
		{
			name:           "heterogeneous A100 + H100 (same profiles, grouped)",
			deviceProfiles: discovery.DeviceProfiles{0: gpuProfiles["A100-80GB"], 1: gpuProfiles["H100-80GB"]},
			wantConfigs: []wantConfig{
				// Balanced - each GPU gets its own entry with device-filter
				{"all-balanced", "1g.10gb", 2, []string{idA100_80GB}},
				{"all-balanced", "2g.20gb", 1, []string{idA100_80GB}},
				{"all-balanced", "3g.40gb", 1, []string{idA100_80GB}},
				{"all-balanced", "1g.10gb", 2, []string{idH100_80GB}},
				{"all-balanced", "2g.20gb", 1, []string{idH100_80GB}},
				{"all-balanced", "3g.40gb", 1, []string{idH100_80GB}},
				// Per-profile - grouped with both device IDs in filter
				{"all-1g.10gb", "1g.10gb", 7, []string{idA100_80GB, idH100_80GB}},
				{"all-1g.10gb.me", "1g.10gb+me", 1, []string{idA100_80GB, idH100_80GB}},
				{"all-1g.20gb", "1g.20gb", 4, []string{idA100_80GB, idH100_80GB}},
				{"all-2g.20gb", "2g.20gb", 3, []string{idA100_80GB, idH100_80GB}},
				{"all-3g.40gb", "3g.40gb", 2, []string{idA100_80GB, idH100_80GB}},
				{"all-4g.40gb", "4g.40gb", 1, []string{idA100_80GB, idH100_80GB}},
				{"all-7g.80gb", "7g.80gb", 1, []string{idA100_80GB, idH100_80GB}},
			},
		},
		{
			name:           "heterogeneous A100 + A30 (different profiles, separate entries)",
			deviceProfiles: discovery.DeviceProfiles{0: gpuProfiles["A100-80GB"], 1: gpuProfiles["A30-24GB"]},
			wantConfigs: []wantConfig{
				// Balanced - A100-80GB (7-slot)
				{"all-balanced", "1g.10gb", 2, []string{idA100_80GB}},
				{"all-balanced", "2g.20gb", 1, []string{idA100_80GB}},
				{"all-balanced", "3g.40gb", 1, []string{idA100_80GB}},
				// Balanced - A30-24GB (4-slot)
				{"all-balanced", "1g.6gb", 2, []string{idA30}},
				{"all-balanced", "2g.12gb", 1, []string{idA30}},
				// Per-profile - A100-80GB
				{"all-1g.10gb", "1g.10gb", 7, []string{idA100_80GB}},
				{"all-1g.10gb.me", "1g.10gb+me", 1, []string{idA100_80GB}},
				{"all-1g.20gb", "1g.20gb", 4, []string{idA100_80GB}},
				{"all-2g.20gb", "2g.20gb", 3, []string{idA100_80GB}},
				{"all-3g.40gb", "3g.40gb", 2, []string{idA100_80GB}},
				{"all-4g.40gb", "4g.40gb", 1, []string{idA100_80GB}},
				{"all-7g.80gb", "7g.80gb", 1, []string{idA100_80GB}},
				// Per-profile - A30-24GB
				{"all-1g.6gb", "1g.6gb", 4, []string{idA30}},
				{"all-1g.6gb.me", "1g.6gb+me", 1, []string{idA30}},
				{"all-2g.12gb", "2g.12gb", 2, []string{idA30}},
				{"all-2g.12gb.me", "2g.12gb+me", 1, []string{idA30}},
				{"all-4g.24gb", "4g.24gb", 1, []string{idA30}},
			},
		},
		{
			name:           "RTX PRO 6000 (4-slot with new attributes)",
			deviceProfiles: discovery.DeviceProfiles{0: gpuProfiles["RTX-PRO-6000"]},
			wantConfigs: []wantConfig{
				// Balanced
				{"all-balanced", "1g.24gb", 2, nil},
				{"all-balanced", "2g.48gb", 1, nil},
				// Per-profile
				{"all-1g.24gb", "1g.24gb", 4, nil},
				{"all-1g.24gb.me", "1g.24gb+me", 1, nil},
				{"all-1g.24gb.gfx", "1g.24gb+gfx", 4, nil},
				{"all-1g.24gb.me.all", "1g.24gb+me.all", 1, nil},
				{"all-1g.24gb-me", "1g.24gb-me", 4, nil},
				{"all-2g.48gb", "2g.48gb", 2, nil},
				{"all-2g.48gb.gfx", "2g.48gb+gfx", 2, nil},
				{"all-2g.48gb.me.all", "2g.48gb+me.all", 1, nil},
				{"all-2g.48gb-me", "2g.48gb-me", 2, nil},
				{"all-4g.96gb", "4g.96gb", 1, nil},
				{"all-4g.96gb.gfx", "4g.96gb+gfx", 1, nil},
			},
		},
		{
			name: "same profile different max counts creates multiple entries",
			deviceProfiles: discovery.DeviceProfiles{
				0: {{Name: "1g.10gb", MaxCount: 7, DeviceID: mockDeviceID(deviceIDA100_80GB), Profile: mockProfile(1, 10, nil, nil)}},
				1: {{Name: "1g.10gb", MaxCount: 4, DeviceID: mockDeviceID(0x20B210DE), Profile: mockProfile(1, 10, nil, nil)}},
			},
			wantConfigs: []wantConfig{
				// No balanced (missing 2g profile), only per-profile
				{"all-1g.10gb", "1g.10gb", 7, []string{idA100_80GB}},
				{"all-1g.10gb", "1g.10gb", 4, []string{idOther}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec, err := buildMigConfigSpec(tc.deviceProfiles)

			require.NoError(t, err)
			require.NotNil(t, spec)

			// Verify base configs
			disabled := spec.MigConfigs["all-disabled"]
			require.Len(t, disabled, 1)
			assert.Equal(t, "all", disabled[0].Devices)
			assert.False(t, disabled[0].MigEnabled)

			enabled := spec.MigConfigs["all-enabled"]
			require.Len(t, enabled, 1)
			assert.Equal(t, "all", enabled[0].Devices)
			assert.True(t, enabled[0].MigEnabled)
			assert.Empty(t, enabled[0].MigDevices)

			// Verify all configs (balanced + per-profile)
			for _, want := range tc.wantConfigs {
				configs, ok := spec.MigConfigs[want.configName]
				require.True(t, ok, "missing config: %s", want.configName)

				// Find the config entry matching this device-filter
				var matched *v1.MigConfigSpec
				for i := range configs {
					cfg := &configs[i]
					if want.deviceFilter == nil && cfg.DeviceFilter == nil {
						matched = cfg
						break
					}
					if filter, ok := cfg.DeviceFilter.([]string); ok {
						if elementsMatch(want.deviceFilter, filter) {
							matched = cfg
							break
						}
					}
				}
				require.NotNil(t, matched, "config %s: no entry with device-filter %v", want.configName, want.deviceFilter)

				assert.Equal(t, "all", matched.Devices)
				assert.True(t, matched.MigEnabled, "config %s should have mig-enabled: true", want.configName)
				assert.Equal(t, want.count, matched.MigDevices[want.profileName], "config %s should have %s: %d", want.configName, want.profileName, want.count)
			}
		})
	}
}
