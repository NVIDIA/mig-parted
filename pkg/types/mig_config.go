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
	"sort"
	"strings"
)

// MigConfig holds a map of strings representing a MigProfile to a count of that profile type.
// It is meant to represent the set of MIG profiles (and how many of a
// particular type) should be instantiated on a GPU.
type MigConfig map[string]int

// NewMigConfig creates a new 'MigConfig' from a slice of 'MigProfile's.
func NewMigConfig(mps []*MigProfile) MigConfig {
	config := make(MigConfig)
	for _, mp := range mps {
		config[mp.String()]++
	}
	return config
}

// AssertValid checks to ensure that all of the 'MigProfiles's making up a 'MigConfig' are valid.
func (m MigConfig) AssertValid() error {
	if len(m) == 0 {
		return nil
	}
	for k, v := range m {
		_, err := ParseMigProfile(k)
		if err != nil {
			return fmt.Errorf("invalid format for '%v': %v", k, err)
		}
		if v < 0 {
			return fmt.Errorf("invalid count for '%v': %v", v, err)
		}
	}
	for _, v := range m {
		if v > 0 {
			return nil
		}
	}
	return fmt.Errorf("all counts for all MigProfiles are 0")
}

// IsSubsetOf checks if the provided 'MigConfig' is a subset of the originating 'MigConfig'.
func (m MigConfig) IsSubsetOf(config MigConfig) bool {
	for k, v := range m {
		if v > 0 && !config.Contains(k) {
			return false
		}
		if v > config[k] {
			return false
		}
	}
	return true
}

// Contains checks if the provided 'profile' is part of the 'MigConfig'.
func (m MigConfig) Contains(profile string) bool {
	if _, exists := m[profile]; !exists {
		return false
	}
	return m[profile] > 0
}

// Equals checks if two 'MigConfig's are equal.
// Equality is determined by comparing the profiles contained in each 'MigConfig'.
func (m MigConfig) Equals(config MigConfig) bool {
	if len(m) != len(config) {
		return false
	}
	for k, v := range m {
		if !config.Contains(k) {
			return false
		}
		if v != config[k] {
			return false
		}
	}
	return true
}

// Flatten converts a 'MigConfig' into a slice of 'MigProfile's.
// Duplicate 'MigProfile's will exist in this slice for each profile represented in the 'MigConfig'.
func (m MigConfig) Flatten() []*MigProfile {
	var mps []*MigProfile
	for k, v := range m {
		mp, err := ParseMigProfile(k)
		if err != nil {
			return nil
		}
		if v < 0 {
			return nil
		}
		if v == 0 {
			continue
		}
		for i := 0; i < v; i++ {
			mps = append(mps, mp)
		}
	}
	sort.Slice(mps, func(i, j int) bool {
		if mps[j].G > mps[i].G {
			return false
		}
		if mps[j].G < mps[i].G {
			return true
		}
		if mps[j].C > mps[i].C {
			return false
		}
		if mps[j].C < mps[i].C {
			return true
		}
		return strings.Join(mps[j].Attributes(), ",") < strings.Join(mps[i].Attributes(), ",")
	})
	return mps
}
