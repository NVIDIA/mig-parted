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

package export

import (
	"testing"

	"github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/stretchr/testify/require"
)

func TestMergeConfigSpecs(t *testing.T) {
	testCases := []struct {
		Description string
		Input       v1.MigConfigSpecSlice
		Output      v1.MigConfigSpecSlice
	}{
		{
			"Empty",
			v1.MigConfigSpecSlice{},
			v1.MigConfigSpecSlice{},
		},
		{
			"Single Device",
			v1.MigConfigSpecSlice{
				{
					DeviceFilter: []string{"A100-SXM4-40GB"},
					Devices:      []int{0},
					MigEnabled:   false,
				},
			},
			v1.MigConfigSpecSlice{
				{
					Devices:    "all",
					MigEnabled: false,
				},
			},
		},
		{
			"Single Filter - Multi Device - Same Config",
			v1.MigConfigSpecSlice{
				{
					DeviceFilter: []string{"A100-SXM4-40GB"},
					Devices:      []int{0},
					MigEnabled:   false,
				},
				{
					DeviceFilter: []string{"A100-SXM4-40GB"},
					Devices:      []int{1},
					MigEnabled:   false,
				},
			},
			v1.MigConfigSpecSlice{
				{
					Devices:    "all",
					MigEnabled: false,
				},
			},
		},
		{
			"Single Filter - Multi Device - Different Config",
			v1.MigConfigSpecSlice{
				{
					DeviceFilter: []string{"A100-SXM4-40GB"},
					Devices:      []int{0},
					MigEnabled:   false,
				},
				{
					DeviceFilter: []string{"A100-SXM4-40GB"},
					Devices:      []int{1},
					MigEnabled:   true,
				},
			},
			v1.MigConfigSpecSlice{
				{
					Devices:    []int{0},
					MigEnabled: false,
				},
				{
					Devices:    []int{1},
					MigEnabled: true,
				},
			},
		},
		{
			"Multi Filter - Same Config",
			v1.MigConfigSpecSlice{
				{
					DeviceFilter: []string{"A100-SXM4-40GB"},
					Devices:      []int{0},
					MigEnabled:   false,
				},
				{
					DeviceFilter: []string{"A100-SXM4-80GB"},
					Devices:      []int{1},
					MigEnabled:   false,
				},
			},
			v1.MigConfigSpecSlice{
				{
					Devices:    "all",
					MigEnabled: false,
				},
			},
		},
		{
			"Multi Filter - Same config per filter",
			v1.MigConfigSpecSlice{
				{
					DeviceFilter: []string{"A100-SXM4-40GB"},
					Devices:      []int{0},
					MigEnabled:   false,
				},
				{
					DeviceFilter: []string{"A100-SXM4-40GB"},
					Devices:      []int{1},
					MigEnabled:   false,
				},
				{
					DeviceFilter: []string{"A100-SXM4-80GB"},
					Devices:      []int{2},
					MigEnabled:   true,
				},
				{
					DeviceFilter: []string{"A100-SXM4-80GB"},
					Devices:      []int{3},
					MigEnabled:   true,
				},
			},
			v1.MigConfigSpecSlice{
				{
					DeviceFilter: "A100-SXM4-40GB",
					Devices:      "all",
					MigEnabled:   false,
				},
				{
					DeviceFilter: "A100-SXM4-80GB",
					Devices:      "all",
					MigEnabled:   true,
				},
			},
		},
		{
			"Multi Filter - Different config per filter - Common config across devices",
			v1.MigConfigSpecSlice{
				{
					DeviceFilter: []string{"A100-SXM4-40GB"},
					Devices:      []int{0},
					MigEnabled:   false,
				},
				{
					DeviceFilter: []string{"A100-SXM4-40GB"},
					Devices:      []int{1},
					MigEnabled:   true,
				},
				{
					DeviceFilter: []string{"A100-SXM4-80GB"},
					Devices:      []int{2},
					MigEnabled:   false,
				},
				{
					DeviceFilter: []string{"A100-SXM4-80GB"},
					Devices:      []int{3},
					MigEnabled:   true,
				},
			},
			v1.MigConfigSpecSlice{
				{
					DeviceFilter: []string{"A100-SXM4-40GB", "A100-SXM4-80GB"},
					Devices:      []int{0, 2},
					MigEnabled:   false,
				},
				{
					DeviceFilter: []string{"A100-SXM4-40GB", "A100-SXM4-80GB"},
					Devices:      []int{1, 3},
					MigEnabled:   true,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			merged := mergeMigConfigSpecs(tc.Input)
			require.Equal(t, tc.Output, merged)
		})
	}
}
