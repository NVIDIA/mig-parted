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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMigProfile(t *testing.T) {
	testCases := []struct {
		description string
		device      string
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
			"Valid 1g.5gb+me",
			"1g.5gb+me",
			true,
		},
		{
			"Valid 1c.1g.5gb+me",
			"1c.1g.5gb+me",
			true,
		},
		{
			"Invalid 0g.0gb",
			"0g.0gb",
			false,
		},
		{
			"Invalid 0c.0g.0gb",
			"0c.0g.0gb",
			false,
		},
		{
			"Invalid 10000g.500000gb",
			"10000g.500000gb",
			false,
		},
		{
			"Invalid 10000c.10000g.500000gb",
			"10000c.10000g.500000gb",
			false,
		},
		{
			"Invalid ' 1c.1g.5gb'",
			" 1c.1g.5gb",
			false,
		},
		{
			"Invalid '1 c.1g.5gb'",
			"1 c.1g.5gb",
			false,
		},
		{
			"Invalid '1c .1g.5gb'",
			"1c .1g.5gb",
			false,
		},
		{
			"Invalid '1c. 1g.5gb'",
			"1c. 1g.5gb",
			false,
		},
		{
			"Invalid '1c.1 g.5gb'",
			"1c.1 g.5gb",
			false,
		},
		{
			"Invalid '1c.1g .5gb'",
			"1c.1g .5gb",
			false,
		},
		{
			"Invalid '1c.1g. 5gb'",
			"1c.1g. 5gb",
			false,
		},
		{
			"Invalid '1c.1g.5 gb'",
			"1c.1g.5 gb",
			false,
		},
		{
			"Invalid '1c.1g.5g b'",
			"1c.1g.5g b",
			false,
		},
		{
			"Invalid '1c.1g.5gb '",
			"1c.1g.5gb ",
			false,
		},
		{
			"Invalid '1c . 1g . 5gb'",
			"1c . 1g . 5gb",
			false,
		},
		{
			"Invalid 1c.f1g.5gb",
			"1c.f1g.5gb",
			false,
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
			"Invalid 1g.5gb+me,me",
			"1g.5gb+me,me",
			false,
		},
		{
			"Invalid 1g.5gb+me,you,them",
			"1g.5gb+me,you,them",
			false,
		},
		{
			"Invalid 1c.1g.5gb+me,you,them",
			"1c.1g.5gb+me,you,them",
			false,
		},
		{
			"Invalid 1g.5gb+",
			"1g.5gb+",
			false,
		},
		{
			"Invalid 1g.5gb +",
			"1g.5gb+",
			false,
		},
		{
			"Invalid 1g.5gb+ ",
			"1g.5gb+",
			false,
		},
		{
			"Invalid 1g.5gb+ ,",
			"1g.5gb+",
			false,
		},
		{
			"Invalid 1g.5gb+,",
			"1g.5gb+",
			false,
		},
		{
			"Invalid 1g.5gb+,,",
			"1g.5gb+",
			false,
		},
		{
			"Invalid 1g.5gb+me,",
			"1g.5gb+",
			false,
		},
		{
			"Invalid 1g.5gb+me,,",
			"1g.5gb+",
			false,
		},
		{
			"Invalid 1g.5gb+me, ",
			"1g.5gb+",
			false,
		},
		{
			"Invalid 1g.5gb+2me",
			"1g.5gb+2me",
			false,
		},
		{
			"Inavlid 1g.5gb*me",
			"1g.5gb*me",
			false,
		},
		{
			"Invalid 1c.1g.5gb*me",
			"1c.1g.5gb*me",
			false,
		},
		{
			"Invalid 1g.5gb*me,you,them",
			"1g.5gb*me,you,them",
			false,
		},
		{
			"Invalid 1c.1g.5gb*me,you,them",
			"1c.1g.5gb*me,you,them",
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
			_, err := ParseMigProfile(tc.device)
			if tc.valid {
				require.Nil(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestGetMigMemorySizeInGB(t *testing.T) {
	type testCase struct {
		totalDeviceMemory    uint64
		migMemorySizeMB      uint64
		expectedMemorySizeGB uint64
	}

	const maxMemorySlices = 8
	const oneMB = uint64(1024 * 1024)
	const oneGB = uint64(1024 * 1024 * 1024)

	totalDeviceMemory := []uint64{
		24 * oneGB,
		40 * oneGB,
		80 * oneGB,
	}

	testCases := []testCase{}
	for _, tdm := range totalDeviceMemory {
		sliceSize := tdm / maxMemorySlices

		const stepSize = oneGB / 32
		for i := stepSize; i <= tdm; i += stepSize {
			tc := testCase{
				totalDeviceMemory: tdm,
				migMemorySizeMB:   i / oneMB,
			}
			for j := uint64(sliceSize); j <= tdm; j += sliceSize {
				if i <= j {
					tc.expectedMemorySizeGB = j / oneGB
					break
				}
			}
			testCases = append(testCases, tc)
		}
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%v", tc.migMemorySizeMB), func(t *testing.T) {
			memorySizeGB := getMigMemorySizeInGB(tc.totalDeviceMemory, tc.migMemorySizeMB)
			require.Equal(t, int(tc.expectedMemorySizeGB), int(memorySizeGB))
		})
	}
}
