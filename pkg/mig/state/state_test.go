/*
 * Copyright (c) 2022, NVIDIA CORPORATION.  All rights reserved.
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

package state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock/dgxa100"

	"github.com/NVIDIA/mig-parted/pkg/mig/config"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

func newMockMigStateManagerOnLunaServer() *migStateManager {
	nvml := dgxa100.New()
	return NewMockMigStateManager(nvml).(*migStateManager)
}

func TestFetchRestore(t *testing.T) {
	manager := newMockMigStateManagerOnLunaServer()

	numGPUs, ret := manager.nvml.DeviceGetCount()
	require.NotNil(t, ret, "Unexpected nil return from DeviceGetCount")
	require.Equal(t, ret, nvml.SUCCESS, "Unexpected return value from DeviceGetCount")

	mcg := config.NewA100_SXM4_40GB_MigConfigGroup()

	type testCase struct {
		description string
		mode        mode.MigMode
		config      types.MigConfig
	}
	testCases := func() []testCase {
		testCases := []testCase{
			{
				"Disabled",
				mode.Disabled,
				nil,
			},
			{
				"Enabled, Empty",
				mode.Enabled,
				nil,
			},
		}
		for _, mc := range mcg.GetPossibleConfigurations() {
			tc := testCase{
				fmt.Sprintf("%v", mc.Flatten()),
				mode.Enabled,
				mc,
			}
			testCases = append(testCases, tc)
		}
		return testCases
	}()

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			for i := 0; i < numGPUs; i++ {
				err := manager.mode.SetMigMode(i, tc.mode)
				require.Nil(t, err)

				if tc.mode == mode.Enabled {
					err = manager.config.SetMigConfig(i, tc.config)
					require.Nil(t, err, "Unexpected failure from SetMigConfig")
				}

				state0, err := manager.Fetch()
				require.Nil(t, err)

				err = manager.RestoreMode(state0)
				require.Nil(t, err)

				err = manager.RestoreConfig(state0)
				require.Nil(t, err)

				state1, err := manager.Fetch()
				require.Nil(t, err)

				require.Equal(t, state0, state1)
			}
		})
	}
}
