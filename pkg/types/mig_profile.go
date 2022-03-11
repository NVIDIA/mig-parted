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
	"strconv"
	"strings"

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
	_, _, _, err := m.Parse()
	if err != nil {
		return fmt.Errorf("error parsing MIG profile: %v", err)
	}
	return nil
}

func parseMigProfilePart(s string, field string) (int, error) {
	if strings.TrimSpace(s) != s {
		return -1, fmt.Errorf("leading or trailing spaces on '%%d%s'", field)
	}

	if !strings.HasSuffix(s, field) {
		return -1, fmt.Errorf("missing '%s' from '%%d%s'", field, field)
	}

	v, err := strconv.Atoi(strings.TrimSuffix(s, field))
	if err != nil {
		return -1, fmt.Errorf("malformed number in '%%d%s'", field)
	}

	return v, nil
}

// Parse breaks a MigProfile into its constituent parts
func (m MigProfile) Parse() (int, int, int, error) {
	var err error
	var c, g, gb int

	split := strings.SplitN(string(m), ".", 3)
	if len(split) == 3 {
		c, err = parseMigProfilePart(split[0], "c")
		if err != nil {
			return -1, -1, -1, fmt.Errorf("invalid MigProfile: %v", err)
		}
		g, err = parseMigProfilePart(split[1], "g")
		if err != nil {
			return -1, -1, -1, fmt.Errorf("invalid MigProfile: %v", err)
		}
		gb, err = parseMigProfilePart(split[2], "gb")
		if err != nil {
			return -1, -1, -1, fmt.Errorf("invalid MigProfile: %v", err)
		}
		return c, g, gb, nil
	}
	if len(split) == 2 {
		g, err = parseMigProfilePart(split[0], "g")
		if err != nil {
			return -1, -1, -1, fmt.Errorf("invalid MigProfile: %v", err)
		}
		gb, err = parseMigProfilePart(split[1], "gb")
		if err != nil {
			return -1, -1, -1, fmt.Errorf("invalid MigProfile: %v", err)
		}
		return g, g, gb, nil
	}

	return -1, -1, -1, fmt.Errorf("invalid MigProfile: parsed wrong number of values, expected 2 or 3")
}

// MustParse breaks a MigProfile into its constituent parts
func (m MigProfile) MustParse() (int, int, int) {
	c, g, gb, _ := m.Parse()
	return c, g, gb
}

// Normalize normalizes a MigProfile to its canonical name
func (m MigProfile) Normalize() (MigProfile, error) {
	c, g, gb, err := m.Parse()
	if err != nil {
		return "", fmt.Errorf("unable to normalize MigProfile: %v", err)
	}
	return NewMigProfile(uint32(c), uint32(g), uint64(gb*1024)), nil
}

// MustNormalize normalizes a MigProfile to its canonical name
func (m MigProfile) MustNormalize() MigProfile {
	normalized, _ := m.Normalize()
	return normalized
}

// Equals checks if two MigProfiles are identical or not
func (m MigProfile) Equals(other MigProfile) bool {
	return m.MustNormalize() == other.MustNormalize()
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
