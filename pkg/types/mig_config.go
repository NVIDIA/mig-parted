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

// MigConfig holds a map of MigProfile to a count of that profile type.
// It is meant to represent the set of MIG profiles (and how many of a
// particular type) should be instantiated on a GPU.
type MigConfig map[MigProfile]int

func NewMigConfig(mps []MigProfile) MigConfig {
	config := make(MigConfig)
	for _, mp := range mps {
		config[mp.MustNormalize()] += 1
	}
	return config
}

func (m MigConfig) AssertValid() error {
	if len(m) == 0 {
		return nil
	}
	for k, v := range m {
		err := k.AssertValid()
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

func (m MigConfig) Contains(profile MigProfile) bool {
	if _, exists := m[profile]; !exists {
		return false
	}
	return m[profile] > 0
}

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

func (m MigConfig) Flatten() []MigProfile {
	var mps []MigProfile
	for k, v := range m {
		if k.AssertValid() != nil {
			return nil
		}
		if v < 0 {
			return nil
		}
		if v == 0 {
			continue
		}
		for i := 0; i < v; i++ {
			mps = append(mps, k)
		}
	}
	sort.Slice(mps, func(i, j int) bool {
		ci, gi, _, attri, _ := mps[i].Parse()
		cj, gj, _, attrj, _ := mps[j].Parse()
		if gj > gi {
			return false
		}
		if gj < gi {
			return true
		}
		if cj > ci {
			return false
		}
		if cj < ci {
			return true
		}
		return strings.Join(attrj, ",") < strings.Join(attri, ",")
	})
	return mps
}
