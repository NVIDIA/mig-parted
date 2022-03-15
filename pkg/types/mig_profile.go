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

const (
	AttributeMediaExtensions = "me"
)

// MigProfile reprents a specific MIG profile name.
// Examples include "1g.5gb" or "2g.10gb" or "1c.2g.10gb", etc.
type MigProfile string

// NewMigProfile constructs a new MigProfile from its constituent parts.
func NewMigProfile(c uint32, g uint32, mb uint64, attr ...string) MigProfile {
	var suffix string
	if len(attr) > 0 {
		suffix = "+" + strings.Join(attr, ",")
	}
	gb := ((mb + 1024 - 1) / 1024)
	if c == g {
		return MigProfile(fmt.Sprintf("%dg.%dgb%s", g, gb, suffix))
	}
	return MigProfile(fmt.Sprintf("%dc.%dg.%dgb%s", c, g, gb, suffix))
}

func (m MigProfile) AddAttributes(giProfileId, ciProfileId, ciEngProfileId int) MigProfile {
	c, g, gb, attr := m.MustParse()
	switch giProfileId {
	case nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1:
		if !m.HasAttribute(AttributeMediaExtensions) {
			attr = append(attr, AttributeMediaExtensions)
		}
	}
	return NewMigProfile(uint32(c), uint32(g), uint64(gb)*1024, attr...)
}

func (m MigProfile) HasAttribute(attr string) bool {
	_, _, _, attrs := m.MustParse()
	for _, a := range attrs {
		if a == attr {
			return true
		}
	}
	return false
}

// AssertValid asserts that a given MigProfile is formatted correctly.
func (m MigProfile) AssertValid() error {
	_, _, _, _, err := m.Parse()
	if err != nil {
		return fmt.Errorf("error parsing MIG profile: %v", err)
	}
	return nil
}

func parseMigProfileField(s string, field string) (int, error) {
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

func parseMigProfileFields(s string) (int, int, int, error) {
	var err error
	var c, g, gb int

	split := strings.SplitN(s, ".", 3)
	if len(split) == 3 {
		c, err = parseMigProfileField(split[0], "c")
		if err != nil {
			return -1, -1, -1, err
		}
		g, err = parseMigProfileField(split[1], "g")
		if err != nil {
			return -1, -1, -1, err
		}
		gb, err = parseMigProfileField(split[2], "gb")
		if err != nil {
			return -1, -1, -1, err
		}
		return c, g, gb, err
	}
	if len(split) == 2 {
		g, err = parseMigProfileField(split[0], "g")
		if err != nil {
			return -1, -1, -1, err
		}
		gb, err = parseMigProfileField(split[1], "gb")
		if err != nil {
			return -1, -1, -1, err
		}
		return g, g, gb, nil
	}

	return -1, -1, -1, fmt.Errorf("parsed wrong number of fields, expected 2 or 3")
}

func parseMigProfileAttributes(s string) ([]string, error) {
	attr := strings.Split(s, ",")
	if len(attr) == 0 {
		return nil, fmt.Errorf("empty attribute list")
	}
	for _, a := range attr {
		if a == "" {
			return nil, fmt.Errorf("empty attribute in list")
		}
		if strings.TrimSpace(a) != a {
			return nil, fmt.Errorf("leading or trailing spaces in attribute")
		}
		if a[0] >= '0' && a[0] <= '9' {
			return nil, fmt.Errorf("attribute begins with a number")
		}
		for _, c := range a {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
				return nil, fmt.Errorf("non alpha-numeric character or digit in attribute")
			}
		}
	}
	return attr, nil
}

// Parse breaks a MigProfile into its constituent parts
func (m MigProfile) Parse() (int, int, int, []string, error) {
	var err error
	var c, g, gb int
	var attr []string

	if len(m) == 0 {
		return -1, -1, -1, nil, fmt.Errorf("empty MigProfile string")
	}

	split := strings.SplitN(string(m), "+", 2)
	if len(split) == 2 {
		attr, err = parseMigProfileAttributes(split[1])
		if err != nil {
			return -1, -1, -1, nil, fmt.Errorf("error parsing attributes following '+' in MigProfile string: %v", err)
		}
	}

	c, g, gb, err = parseMigProfileFields(split[0])
	if err != nil {
		return -1, -1, -1, nil, fmt.Errorf("error parsing '.' separated fields in MigProfile string: %v", err)
	}

	return c, g, gb, attr, nil
}

// MustParse breaks a MigProfile into its constituent parts
func (m MigProfile) MustParse() (int, int, int, []string) {
	c, g, gb, attr, _ := m.Parse()
	return c, g, gb, attr
}

// Normalize normalizes a MigProfile to its canonical name
func (m MigProfile) Normalize() (MigProfile, error) {
	c, g, gb, attr, err := m.Parse()
	if err != nil {
		return "", fmt.Errorf("unable to normalize MigProfile: %v", err)
	}
	return NewMigProfile(uint32(c), uint32(g), uint64(gb)*1024, attr...), nil
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

	c, g, _, _, err := m.Parse()
	if err != nil {
		return -1, -1, -1, fmt.Errorf("unable to parse MigProfile: %v", err)
	}

	var giProfileID, ciProfileID, ciEngProfileID int

	switch g {
	case 1:
		giProfileID = nvml.GPU_INSTANCE_PROFILE_1_SLICE
		if m.HasAttribute(AttributeMediaExtensions) {
			giProfileID = nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1
		}
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
