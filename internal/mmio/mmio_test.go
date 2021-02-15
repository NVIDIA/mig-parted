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

package mmio

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMmioRead(t *testing.T) {
	source := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

	type testCase struct {
		description string
		offset      int
	}
	testCases := func() []testCase {
		var testCases []testCase
		for i := 0; i < len(source)/2; i++ {
			tc := testCase{
				fmt.Sprintf("offset: %v", i),
				i,
			}
			testCases = append(testCases, tc)
		}
		return testCases
	}()

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			mmio, err := MockOpenRO(&source, 0, len(source))
			require.Nil(t, err, "Unexpected error from OpenRO")

			defer func() {
				err = mmio.Close()
				require.Nil(t, err, "Unexpected error from Close")
			}()

			r8 := mmio.Read8(tc.offset)
			require.Equal(t, r8, source[tc.offset])

			r16 := mmio.Read16(tc.offset)
			require.Equal(t, r16, binary.LittleEndian.Uint16(source[tc.offset:tc.offset+2]))

			r32 := mmio.Read32(tc.offset)
			require.Equal(t, r32, binary.LittleEndian.Uint32(source[tc.offset:tc.offset+4]))

			r64 := mmio.Read64(tc.offset)
			require.Equal(t, r64, binary.LittleEndian.Uint64(source[tc.offset:tc.offset+8]))
		})
	}
}

func TestMmioWrite(t *testing.T) {
	source := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

	type testCase struct {
		description string
		offset      int
	}
	testCases := func() []testCase {
		var testCases []testCase
		for i := 0; i < len(source)/2; i++ {
			tc := testCase{
				fmt.Sprintf("offset: %v", i),
				i,
			}
			testCases = append(testCases, tc)
		}
		return testCases
	}()

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			mmio, err := MockOpenRW(&source, 0, len(source))
			require.Nil(t, err, "Unexpected error from OpenRW")

			defer func() {
				err = mmio.Close()
				require.Nil(t, err, "Unexpected error from Close")
			}()

			r8 := mmio.Read8(tc.offset)
			mmio.Write8(tc.offset, (1<<8)-1)
			require.Equal(t, r8, source[tc.offset])
			r8 = mmio.Read8(tc.offset)
			require.Equal(t, r8, uint8((1<<8)-1))
			mmio.Sync()
			require.Equal(t, r8, source[tc.offset])
			mmio.Write8(tc.offset, uint8(tc.offset))
			mmio.Sync()

			r16 := mmio.Read16(tc.offset)
			mmio.Write16(tc.offset, (1<<16)-1)
			require.Equal(t, r16, binary.LittleEndian.Uint16(source[tc.offset:tc.offset+2]))
			r16 = mmio.Read16(tc.offset)
			require.Equal(t, r16, uint16((1<<16)-1))
			mmio.Sync()
			require.Equal(t, r16, binary.LittleEndian.Uint16(source[tc.offset:tc.offset+2]))
			mmio.Write8(tc.offset+0, uint8(tc.offset+0))
			mmio.Write8(tc.offset+1, uint8(tc.offset+1))
			mmio.Sync()

			r32 := mmio.Read32(tc.offset)
			mmio.Write32(tc.offset, (1<<32)-1)
			require.Equal(t, r32, binary.LittleEndian.Uint32(source[tc.offset:tc.offset+4]))
			r32 = mmio.Read32(tc.offset)
			require.Equal(t, r32, uint32((1<<32)-1))
			mmio.Sync()
			require.Equal(t, r32, binary.LittleEndian.Uint32(source[tc.offset:tc.offset+4]))
			mmio.Write8(tc.offset+0, uint8(tc.offset+0))
			mmio.Write8(tc.offset+1, uint8(tc.offset+1))
			mmio.Write8(tc.offset+2, uint8(tc.offset+2))
			mmio.Write8(tc.offset+3, uint8(tc.offset+3))
			mmio.Sync()

			r64 := mmio.Read64(tc.offset)
			mmio.Write64(tc.offset, (1<<64)-1)
			require.Equal(t, r64, binary.LittleEndian.Uint64(source[tc.offset:tc.offset+8]))
			r64 = mmio.Read64(tc.offset)
			require.Equal(t, r64, uint64((1<<64)-1))
			mmio.Sync()
			require.Equal(t, r64, binary.LittleEndian.Uint64(source[tc.offset:tc.offset+8]))
			mmio.Write8(tc.offset+0, uint8(tc.offset+0))
			mmio.Write8(tc.offset+1, uint8(tc.offset+1))
			mmio.Write8(tc.offset+2, uint8(tc.offset+2))
			mmio.Write8(tc.offset+3, uint8(tc.offset+3))
			mmio.Write8(tc.offset+4, uint8(tc.offset+4))
			mmio.Write8(tc.offset+5, uint8(tc.offset+5))
			mmio.Write8(tc.offset+6, uint8(tc.offset+6))
			mmio.Write8(tc.offset+7, uint8(tc.offset+7))
			mmio.Sync()
		})
	}
}
