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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigProfileAssertValid(t *testing.T) {
	testCases := []struct {
		description string
		device      MigProfile
		valid       bool
	}{
		{
			"Empty device type",
			"",
			false,
		},
		{
			"Valid 1g.5gb",
			"1g.5gb",
			true,
		},
		{
			"Valid 1c.1g.5gb",
			"1c.1g.5gb",
			true,
		},
		{
			"Valid 10000g.500000gb",
			"10000g.500000gb",
			true,
		},
		{
			"Valid 10000c.10000g.500000gb",
			"10000c.10000g.500000gb",
			true,
		},
		{
			"Valid 0g.0gb",
			"0g.0gb",
			true,
		},
		{
			"Valid 0c.0g.0gb",
			"0c.0g.0gb",
			true,
		},
		{
			"Invalid 1r.1g.5gb",
			"1r.1g.5gb",
			false,
		},
		{
			"Invalid 1g.5gbk",
			"1g.5gbk",
			false,
		},
		{
			"Invalid 1g.5",
			"1g.5",
			false,
		},
		{
			"Invalid g.5gb",
			"1g.5",
			false,
		},
		{
			"Invalid g.5gb",
			"g.5gb",
			false,
		},
		{
			"Invalid 1g.gb",
			"1g.gb",
			false,
		},
		{
			"Invalid bogus",
			"bogus",
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			err := tc.device.AssertValid()
			if tc.valid {
				require.Nil(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
