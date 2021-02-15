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

package v1

import (
	"sort"

	"github.com/NVIDIA/mig-parted/pkg/types"
)

func (specs MigConfigSpecSlice) Normalize() MigConfigSpecSlice {
	dfCounts := make(map[string]int)
	for _, s := range specs {
		dfCounts[s.DeviceFilter]++
	}

	var merged []MigConfigSpec
OUTER:
	for _, s := range specs {
		if len(merged) == 0 {
			merged = append(merged, s)
			continue
		}
		for i, m := range merged {
			if s.DeviceFilter != m.DeviceFilter {
				continue
			}
			if s.MigEnabled != m.MigEnabled {
				continue
			}
			if !s.MigDevices.Equals(m.MigDevices) {
				continue
			}
			switch devices := s.Devices.(type) {
			case []int:
				merged[i].Devices = append(m.Devices.([]int), devices...)
				sort.Ints(merged[i].Devices.([]int))
				if len(merged[i].Devices.([]int)) == dfCounts[merged[i].DeviceFilter] {
					merged[i].Devices = "all"
				}
			}
			continue OUTER
		}
		merged = append(merged, s)
	}

	if len(dfCounts) == 1 {
		for i := range merged {
			merged[i].DeviceFilter = ""
		}
	}

	return merged
}

func (ms *MigConfigSpec) MatchesDeviceFilter(deviceID types.DeviceID) bool {
	if ms.DeviceFilter == "" {
		return true
	}

	newDeviceID, _ := types.NewDeviceIDFromString(ms.DeviceFilter)
	if newDeviceID == deviceID {
		return true
	}

	return false
}

func (ms *MigConfigSpec) MatchesDevices(index int) bool {
	switch devices := ms.Devices.(type) {
	case string:
		if devices == "all" {
			return true
		}
		return false
	case []int:
		for _, d := range devices {
			if index == d {
				return true
			}
		}
		return false
	}
	return false
}
