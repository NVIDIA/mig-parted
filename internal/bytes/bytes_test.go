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

package bytes

import (
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	source := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

	bytes := New(&source)
	require.IsType(t, (*native)(&source), bytes)

	bytes = NewLittleEndian(&source)
	if nativeByteOrder == binary.LittleEndian {
		require.IsType(t, (*native)(&source), bytes)
	} else {
		require.IsType(t, (*swapbo)(&source), bytes)

	}

	bytes = NewBigEndian(&source)
	if nativeByteOrder == binary.BigEndian {
		require.IsType(t, (*native)(&source), bytes)
	} else {
		require.IsType(t, (*swapbo)(&source), bytes)
	}
}

func TestRead(t *testing.T) {
	source := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

	type testCase struct {
		Description string
		New         func(*[]byte) Bytes
		ByteOrder   binary.ByteOrder
		Offset      int
	}
	testCases := func() []testCase {
		var testCases []testCase
		for i := 0; i < len(source)/2; i++ {
			tc1 := testCase{
				fmt.Sprintf("LittleEndian offset: %v", i),
				NewLittleEndian,
				binary.LittleEndian,
				i,
			}

			tc2 := testCase{
				fmt.Sprintf("BigEndian offset: %v", i),
				NewBigEndian,
				binary.BigEndian,
				i,
			}

			testCases = append(testCases, tc1, tc2)
		}
		return testCases
	}()

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			bytes := tc.New(&source)

			r8 := bytes.Read8(tc.Offset)
			require.Equal(t, r8, source[tc.Offset])

			r16 := bytes.Read16(tc.Offset)
			require.Equal(t, r16, tc.ByteOrder.Uint16(source[tc.Offset:tc.Offset+2]))

			r32 := bytes.Read32(tc.Offset)
			require.Equal(t, r32, tc.ByteOrder.Uint32(source[tc.Offset:tc.Offset+4]))

			r64 := bytes.Read64(tc.Offset)
			require.Equal(t, r64, tc.ByteOrder.Uint64(source[tc.Offset:tc.Offset+8]))
		})
	}
}

func TestWrite(t *testing.T) {
	source := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

	type testCase struct {
		Description string
		New         func(*[]byte) Bytes
		ByteOrder   binary.ByteOrder
		Offset      int
	}
	testCases := func() []testCase {
		var testCases []testCase
		for i := 0; i < len(source)/2; i++ {
			tc1 := testCase{
				fmt.Sprintf("LittleEndian offset: %v", i),
				NewLittleEndian,
				binary.LittleEndian,
				i,
			}

			tc2 := testCase{
				fmt.Sprintf("BigEndian offset: %v", i),
				NewBigEndian,
				binary.BigEndian,
				i,
			}

			testCases = append(testCases, tc1, tc2)
		}
		return testCases
	}()

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			bytes := tc.New(&source)

			r8 := bytes.Read8(tc.Offset)
			require.Equal(t, r8, source[tc.Offset])
			bytes.Write8(tc.Offset, (1<<8)-1)
			r8 = bytes.Read8(tc.Offset)
			require.Equal(t, r8, uint8((1<<8)-1))
			require.Equal(t, r8, source[tc.Offset])
			bytes.Write8(tc.Offset, uint8(tc.Offset))

			r16 := bytes.Read16(tc.Offset)
			require.Equal(t, r16, tc.ByteOrder.Uint16(source[tc.Offset:tc.Offset+2]))
			bytes.Write16(tc.Offset, (1<<16)-1)
			r16 = bytes.Read16(tc.Offset)
			require.Equal(t, r16, uint16((1<<16)-1))
			require.Equal(t, r16, tc.ByteOrder.Uint16(source[tc.Offset:tc.Offset+2]))
			bytes.Write8(tc.Offset+0, uint8(tc.Offset+0))
			bytes.Write8(tc.Offset+1, uint8(tc.Offset+1))

			r32 := bytes.Read32(tc.Offset)
			require.Equal(t, r32, tc.ByteOrder.Uint32(source[tc.Offset:tc.Offset+4]))
			bytes.Write32(tc.Offset, (1<<32)-1)
			r32 = bytes.Read32(tc.Offset)
			require.Equal(t, r32, uint32((1<<32)-1))
			require.Equal(t, r32, tc.ByteOrder.Uint32(source[tc.Offset:tc.Offset+4]))
			bytes.Write8(tc.Offset+0, uint8(tc.Offset+0))
			bytes.Write8(tc.Offset+1, uint8(tc.Offset+1))
			bytes.Write8(tc.Offset+2, uint8(tc.Offset+2))
			bytes.Write8(tc.Offset+3, uint8(tc.Offset+3))

			r64 := bytes.Read64(tc.Offset)
			require.Equal(t, r64, tc.ByteOrder.Uint64(source[tc.Offset:tc.Offset+8]))
			bytes.Write64(tc.Offset, (1<<64)-1)
			r64 = bytes.Read64(tc.Offset)
			require.Equal(t, r64, uint64((1<<64)-1))
			require.Equal(t, r64, tc.ByteOrder.Uint64(source[tc.Offset:tc.Offset+8]))
			bytes.Write8(tc.Offset+0, uint8(tc.Offset+0))
			bytes.Write8(tc.Offset+1, uint8(tc.Offset+1))
			bytes.Write8(tc.Offset+2, uint8(tc.Offset+2))
			bytes.Write8(tc.Offset+3, uint8(tc.Offset+3))
			bytes.Write8(tc.Offset+4, uint8(tc.Offset+4))
			bytes.Write8(tc.Offset+5, uint8(tc.Offset+5))
			bytes.Write8(tc.Offset+6, uint8(tc.Offset+6))
			bytes.Write8(tc.Offset+7, uint8(tc.Offset+7))
		})
	}
}
