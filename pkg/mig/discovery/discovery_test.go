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
	"testing"

	nvdev "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock/dgxa100"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/mig-parted/pkg/types"
)

func TestDiscoverProfiles(t *testing.T) {
	// TODO: Add tests for other device types when we have mocks for them
	server := dgxa100.New()
	deviceLib := nvdev.New(server, nvdev.WithVerifySymbols(false))

	d := &discoverer{
		nvmllib:   server,
		deviceLib: deviceLib,
	}

	result, err := d.discoverProfiles()
	require.NoError(t, err)
	require.Len(t, result, 8)

	for _, profiles := range result {
		require.GreaterOrEqual(t, len(profiles), 5)

		for _, p := range profiles {
			require.NotEmpty(t, p.Name)
			require.Greater(t, p.MaxCount, 0)
			require.NotContains(t, p.Name, "c.", "CI profiles should be filtered")
		}
	}
}

func TestIsCIProfile(t *testing.T) {
	testCases := []struct {
		name string
		c, g int
		want bool
	}{
		{"GI profile 1g", 0, 1, false},
		{"GI profile 7g", 0, 7, false},
		{"CI profile 1c.2g", 1, 2, true},
		{"CI profile 3c.7g", 3, 7, true},
		{"full slice c==g", 7, 7, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isCIProfile(tc.c, tc.g))
		})
	}
}

func TestGetHardcodedA30Profiles(t *testing.T) {
	deviceID := types.DeviceID(deviceIDA30)
	profiles := getHardcodedA30Profiles(deviceID)

	require.Len(t, profiles, 5)

	expected := map[string]int{
		"1g.6gb":     4,
		"1g.6gb+me":  1,
		"2g.12gb":    2,
		"2g.12gb+me": 1,
		"4g.24gb":    1,
	}

	for _, p := range profiles {
		assert.Equal(t, expected[p.Name], p.MaxCount, "profile %s", p.Name)
		assert.Equal(t, deviceID, p.DeviceID)
		assert.NotNil(t, p.Profile, "profile %s should have non-nil Profile", p.Name)
	}

	// Verify base profiles have no attributes
	for _, p := range profiles {
		info := p.Profile.GetInfo()
		if p.Name == "1g.6gb" || p.Name == "2g.12gb" || p.Name == "4g.24gb" {
			assert.Empty(t, info.Attributes, "base profile %s should have no attributes", p.Name)
		} else {
			assert.Contains(t, info.Attributes, "me", "profile %s should have 'me' attribute", p.Name)
		}
	}
}
