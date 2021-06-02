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

package config

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/NVIDIA/mig-parted/internal/nvml"
	"github.com/NVIDIA/mig-parted/pkg/types"
	"github.com/stretchr/testify/require"
)

func NewMockLunaServerMigConfigManager() Manager {
	return &nvmlMigConfigManager{nvml.NewMockNVMLOnLunaServer()}
}

func EnableMigMode(manager Manager, gpu int) (nvml.Return, nvml.Return) {
	m := manager.(*nvmlMigConfigManager)
	n := m.nvml.(*nvml.MockLunaServer)
	r1, r2 := n.Devices[gpu].SetMigMode(nvml.DEVICE_MIG_ENABLE)
	return r1, r2
}

func TestGetSetMigConfig(t *testing.T) {
	nvmlLib := nvml.NewMockNVMLOnLunaServer()
	manager := NewMockLunaServerMigConfigManager()

	numGPUs, ret := nvmlLib.DeviceGetCount()
	require.NotNil(t, ret, "Unexpected nil return from DeviceGetCount")
	require.Equal(t, ret.Value(), nvml.SUCCESS, "Unexpected return value from DeviceGetCount")

	mcg := NewA100_SXM4_40GB_MigConfigGroup()

	type testCase struct {
		description string
		config      types.MigConfig
	}
	testCases := func() []testCase {
		var testCases []testCase
		for _, mc := range mcg.GetPossibleConfigurations() {
			tc := testCase{
				fmt.Sprintf("%v", mc.Flatten()),
				mc,
			}
			testCases = append(testCases, tc)
		}
		return testCases
	}()

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			for i := 0; i < numGPUs; i++ {
				r1, r2 := EnableMigMode(manager, i)
				require.Equal(t, nvml.SUCCESS, r1.Value())
				require.Equal(t, nvml.SUCCESS, r2.Value())

				err := manager.SetMigConfig(i, tc.config)
				require.Nil(t, err, "Unexpected failure from SetMigConfig")

				config, err := manager.GetMigConfig(i)
				require.Nil(t, err, "Unexpected failure from GetMigConfig")
				require.Equal(t, tc.config.Flatten(), config.Flatten(), "Retrieved MigConfig different than what was set")
			}
		})
	}
}

func TestClearMigConfig(t *testing.T) {
	mcg := NewA100_SXM4_40GB_MigConfigGroup()

	type testCase struct {
		description string
		config      types.MigConfig
	}
	testCases := func() []testCase {
		var testCases []testCase
		for _, mc := range mcg.GetPossibleConfigurations() {
			tc := testCase{
				fmt.Sprintf("%v", mc.Flatten()),
				mc,
			}
			testCases = append(testCases, tc)
		}
		return testCases
	}()

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			manager := NewMockLunaServerMigConfigManager()

			r1, r2 := EnableMigMode(manager, 0)
			require.Equal(t, nvml.SUCCESS, r1.Value())
			require.Equal(t, nvml.SUCCESS, r2.Value())

			err := manager.SetMigConfig(0, tc.config)
			require.Nil(t, err, "Unexpected failure from SetMigConfig")

			err, _ = manager.ClearMigConfig(0, []types.MigProfile{})
			require.Nil(t, err, "Unexpected failure from ClearMigConfig")

			config, err := manager.GetMigConfig(0)
			require.Nil(t, err, "Unexpected failure from GetMigConfig")
			require.Equal(t, 0, len(config.Flatten()), "Unexpected number of configured MIG profiles")
		})
	}
}

func TestIteratePermutationsUntilSuccess(t *testing.T) {
	factorial := func(n int) int {
		product := 1
		for i := 1; i <= n; i++ {
			product *= i
		}
		return product
	}

	uniquePermutations := func(mc types.MigConfig) int {
		perms := factorial(len(mc.Flatten()))
		for _, v := range mc {
			perms /= factorial(v)
		}
		return perms
	}

	rand.Seed(time.Now().UnixNano())
	mcg := NewA100_SXM4_40GB_MigConfigGroup()

	type testCase struct {
		description  string
		config       types.MigConfig
		successAfter int
	}
	testCases := func() []testCase {
		var testCases []testCase
		for _, mc := range mcg.GetPossibleConfigurations() {
			successAfter := rand.Intn(uniquePermutations(mc)) + 1
			tc := testCase{
				fmt.Sprintf("%v:%v", mc.Flatten(), successAfter),
				mc,
				successAfter, // Random stop between 1 and uniquePermutations
			}
			testCases = append(testCases, tc)
		}
		for _, mc := range mcg.GetPossibleConfigurations() {
			tc := testCase{
				fmt.Sprintf("%v:%v", mc.Flatten(), -1),
				mc,
				-1, // Never stop, so expect failure after uniquePermutations
			}
			testCases = append(testCases, tc)
		}
		return testCases
	}()

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			iteration := 0
			err := iteratePermutationsUntilSuccess(tc.config, func(perm []types.MigProfile) error {
				iteration++
				if iteration == tc.successAfter {
					return nil
				}
				err := fmt.Errorf("Failed iteration: %v", iteration)
				return err
			})
			if err == nil {
				require.Equal(t, tc.successAfter, iteration, "Success on wrong iteration")
			} else {
				require.Equal(t, uniquePermutations(tc.config), iteration, "Failed after wrong number of iterations")
			}
		})
	}
}
