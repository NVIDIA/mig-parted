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
	"testing"

	"github.com/NVIDIA/mig-parted/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestValidConfiguration(t *testing.T) {
	testCases := []struct {
		description string
		gpu         types.DeviceID
		config      types.MigConfig
		valid       bool
	}{
		{
			"Empty config",
			A100_SXM4_40GB,
			types.MigConfig{},
			true,
		},
		{
			"Nil config",
			A100_SXM4_40GB,
			nil,
			true,
		},
		{
			"Invalid device type (with count > 0)",
			A100_SXM4_40GB,
			types.MigConfig{
				"bogus": 1,
			},
			false,
		},
		{
			"Invalid device type (with count == 0)",
			A100_SXM4_40GB,
			types.MigConfig{
				"bogus": 0,
			},
			false,
		},
		{
			"Single device (equal to max)",
			A100_SXM4_40GB,
			types.MigConfig{
				"1g.5gb": 7,
			},
			true,
		},
		{
			"Single device (greater than max)",
			A100_SXM4_40GB,
			types.MigConfig{
				"1g.5gb": 8,
			},
			false,
		},
		{
			"Single device (less than max)",
			A100_SXM4_40GB,
			types.MigConfig{
				"1g.5gb": 6,
			},
			true,
		},
		{
			"Single device (equal to 0)",
			A100_SXM4_40GB,
			types.MigConfig{
				"1g.5gb": 0,
			},
			false,
		},
		{
			"Mix of devices (all at max)",
			A100_SXM4_40GB,
			types.MigConfig{
				"1g.5gb":  3,
				"2g.10gb": 2,
			},
			true,
		},
		{
			"Mix of devices (one smaller than max)",
			A100_SXM4_40GB,
			types.MigConfig{
				"1g.5gb":  2,
				"2g.10gb": 2,
			},
			true,
		},
		{
			"Mix of devices (all smaller than max)",
			A100_SXM4_40GB,
			types.MigConfig{
				"1g.5gb":  2,
				"2g.10gb": 1,
			},
			true,
		},
		{
			"Mix of devices (one greater than max)",
			A100_SXM4_40GB,
			types.MigConfig{
				"1g.5gb":  4,
				"2g.10gb": 2,
			},
			false,
		},
	}

	configs := GetKnownMigConfigGroups()

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := configs[tc.gpu].AssertValidConfiguration(tc.config)
			if tc.valid {
				require.Nil(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
