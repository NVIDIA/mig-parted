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
	"regexp"

	"github.com/NVIDIA/mig-parted/internal/nvml"
)

// MigProfile reprents a specific MIG profile name.
// Examples include "1g.5gb" or "2g.10gb" or "1c.2g.10gb", etc.
type MigProfile string

// NewMigProfile constructs a new MigProfile from its constituent parts.
func NewMigProfile(c uint32, g uint32, mb uint64) MigProfile {
	gb := ((mb + 1024 - 1) / 1024)
	if c == g {
		return MigProfile(fmt.Sprintf("%dg.%dgb", g, gb))
	}
	return MigProfile(fmt.Sprintf("%dc.%dg.%dgb", c, g, gb))
}

// AssertValid asserts that a given MigProfile is formatted correctly.
func (m MigProfile) AssertValid() error {
	match, err := regexp.MatchString(`^[0-9]+g\.[0-9]+gb$`, string(m))
	if err != nil {
		return fmt.Errorf("error running regex: %v", err)
	}
	if match {
		return nil
	}

	match, err = regexp.MatchString(`^[0-9]+c\.[0-9]+g\.[0-9]+gb$`, string(m))
	if err != nil {
		return fmt.Errorf("error running regex: %v", err)
	}
	if match {
		return nil
	}

	return fmt.Errorf("no match for format %%dc.%%dg.%%dgb or %%dg.%%dgb")
}

// Parse breaks a MigProfile into its constituent parts
func (m MigProfile) Parse() (int, int, int, error) {
	err := m.AssertValid()
	if err != nil {
		return -1, -1, -1, fmt.Errorf("invalid MigProfile: %v", err)
	}

	var c, g, gb int
	n, _ := fmt.Sscanf(string(m), "%dc.%dg.%dgb", &c, &g, &gb)
	if n == 3 {
		return c, g, gb, nil
	}

	n, _ = fmt.Sscanf(string(m), "%dg.%dgb", &g, &gb)
	if n == 2 {
		return g, g, gb, nil
	}

	return -1, -1, -1, fmt.Errorf("parsed wrong number of values, expected 2 or 3")
}

// GetProfileIDs returns the relevant GI and CI profile IDs for the MigProfile
// These profile IDs are suitable for passing to the relevant NVML calls that require them.
func (m MigProfile) GetProfileIDs() (int, int, int, error) {
	err := m.AssertValid()
	if err != nil {
		return -1, -1, -1, fmt.Errorf("invalid MigProfile: %v", err)
	}

	c, g, _, err := m.Parse()
	if err != nil {
		return -1, -1, -1, fmt.Errorf("unable to parse MigProfile: %v", err)
	}

	var giProfileID, ciProfileID, ciEngProfileID int

	switch g {
	case 1:
		giProfileID = nvml.GPU_INSTANCE_PROFILE_1_SLICE
	case 2:
		giProfileID = nvml.GPU_INSTANCE_PROFILE_2_SLICE
	case 3:
		giProfileID = nvml.GPU_INSTANCE_PROFILE_3_SLICE
	case 4:
		giProfileID = nvml.GPU_INSTANCE_PROFILE_4_SLICE
	case 7:
		giProfileID = nvml.GPU_INSTANCE_PROFILE_7_SLICE
	case 8:
		giProfileID = nvml.GPU_INSTANCE_PROFILE_8_SLICE
	default:
		return -1, -1, -1, fmt.Errorf("unknown GPU Instance slice size: %v", g)
	}

	switch c {
	case 1:
		ciProfileID = nvml.COMPUTE_INSTANCE_PROFILE_1_SLICE
	case 2:
		ciProfileID = nvml.COMPUTE_INSTANCE_PROFILE_2_SLICE
	case 3:
		ciProfileID = nvml.COMPUTE_INSTANCE_PROFILE_3_SLICE
	case 4:
		ciProfileID = nvml.COMPUTE_INSTANCE_PROFILE_4_SLICE
	case 7:
		ciProfileID = nvml.COMPUTE_INSTANCE_PROFILE_7_SLICE
	case 8:
		ciProfileID = nvml.COMPUTE_INSTANCE_PROFILE_8_SLICE
	default:
		return -1, -1, -1, fmt.Errorf("unknown Compute Instance slice size: %v", c)
	}

	ciEngProfileID = nvml.COMPUTE_INSTANCE_ENGINE_PROFILE_SHARED

	return giProfileID, ciProfileID, ciEngProfileID, nil
}
