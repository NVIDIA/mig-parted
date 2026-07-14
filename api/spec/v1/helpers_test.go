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

package v1

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/mig-parted/pkg/types"
)

func TestMigConfigSpecMatchesDeviceFilter(t *testing.T) {
	h100 := types.NewDeviceIDWithSubsystem(0x2330, 0x10DE, 0x16C0, 0x10DE)

	testCases := []struct {
		name   string
		filter interface{}
		want   bool
	}{
		{
			name:   "empty filter matches all devices",
			filter: "",
			want:   true,
		},
		{
			name:   "primary-only filter matches subsystem-aware hardware",
			filter: "0x233010DE",
			want:   true,
		},
		{
			name:   "matching subsystem filter matches hardware",
			filter: "0x233010DE:0x16C010DE",
			want:   true,
		},
		{
			name:   "sibling subsystem filter does not match hardware",
			filter: "0x233010DE:0x16C110DE",
			want:   false,
		},
		{
			name:   "multiple filters stop at a matching subsystem entry",
			filter: []string{"0x233010DE:0x16C110DE", "0x233010DE:0x16C010DE"},
			want:   true,
		},
		{
			name:   "malformed filters with extra separators do not match",
			filter: "0x233010DE:0x16C010DE:extra",
			want:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			spec := MigConfigSpec{
				DeviceFilter: tc.filter,
				Devices:      "all",
			}

			require.Equal(t, tc.want, spec.MatchesDeviceFilter(h100))
		})
	}
}
