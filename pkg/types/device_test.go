/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

func TestNewDeviceID(t *testing.T) {
	require.Equal(t, DeviceID{
		Device: 0x25B6,
		Vendor: 0x10DE,
	}, NewDeviceID(0x25B6, 0x10DE))
}

func TestNewDeviceIDWithSubsystem(t *testing.T) {
	require.Equal(t, DeviceID{
		Device:          0x25B6,
		Vendor:          0x10DE,
		SubsystemDevice: 0x14A9,
		SubsystemVendor: 0x10DE,
		HasSubsystem:    true,
	}, NewDeviceIDWithSubsystem(0x25B6, 0x10DE, 0x14A9, 0x10DE))
}

func TestNewDeviceIDFromString(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		want    DeviceID
		wantErr string
	}{
		{
			name:  "primary only",
			input: "0x25B610DE",
			want:  NewDeviceID(0x25B6, 0x10DE),
		},
		{
			name:  "with subsystem",
			input: "0x25B610DE:0x14A910DE",
			want:  NewDeviceIDWithSubsystem(0x25B6, 0x10DE, 0x14A9, 0x10DE),
		},
		{
			name:    "invalid primary",
			input:   "not-a-device-id",
			wantErr: "unable to create DeviceID",
		},
		{
			name:    "invalid subsystem",
			input:   "0x25B610DE:not-a-subsystem",
			wantErr: "unable to create Subsystem",
		},
		{
			name:    "too many separators",
			input:   "0x25B610DE:0x14A910DE:extra",
			wantErr: "invalid DeviceID format",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewDeviceIDFromString(tc.input)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestDeviceIDString(t *testing.T) {
	require.Equal(t, "0x25B610DE", NewDeviceID(0x25B6, 0x10DE).String())
	require.Equal(
		t,
		"0x25B610DE:0x14A910DE",
		NewDeviceIDWithSubsystem(0x25B6, 0x10DE, 0x14A9, 0x10DE).String(),
	)
}

func TestDeviceIDMatches(t *testing.T) {
	a16 := NewDeviceIDWithSubsystem(0x25B6, 0x10DE, 0x14A9, 0x10DE)
	a2 := NewDeviceIDWithSubsystem(0x25B6, 0x10DE, 0x157E, 0x10DE)

	testCases := []struct {
		name     string
		filter   DeviceID
		hardware DeviceID
		want     bool
	}{
		{
			name:     "primary only filter matches same primary id",
			filter:   NewDeviceID(0x25B6, 0x10DE),
			hardware: a16,
			want:     true,
		},
		{
			name:     "subsystem filter matches exact hardware",
			filter:   NewDeviceIDWithSubsystem(0x25B6, 0x10DE, 0x14A9, 0x10DE),
			hardware: a16,
			want:     true,
		},
		{
			name:     "subsystem filter rejects sibling subsystem",
			filter:   NewDeviceIDWithSubsystem(0x25B6, 0x10DE, 0x14A9, 0x10DE),
			hardware: a2,
			want:     false,
		},
		{
			name:     "different primary id does not match",
			filter:   NewDeviceID(0x1E30, 0x10DE),
			hardware: a16,
			want:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.filter.Matches(tc.hardware))
		})
	}
}
