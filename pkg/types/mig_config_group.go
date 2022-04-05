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
)

// MigConfigGroup provides an interface for intaracting with a logical group of 'MigConfig's.
// One such group is the set of all valid MigConfigs for the A100, for example.
type MigConfigGroup interface {
	GetDeviceTypes() []*MigProfile
	GetPossibleConfigurations() []MigConfig
	AssertValidConfiguration(MigConfig) error
}

// MigConfigGroupBase is a base struct for constructing a full 'MigConfigGroup'.
// It holds the slice of 'MigConfig' structs associated with the group.
type MigConfigGroupBase struct {
	Configs []MigConfig
}

// MigConfigGroups holds the mapping from a specific 'DeviceID' to a 'MigConfigGroup'.
type MigConfigGroups map[DeviceID]MigConfigGroup

// GetPossibleConfigurations gets all possible configurations associated with a 'MigConfigGroup'.
func (m *MigConfigGroupBase) GetPossibleConfigurations() []MigConfig {
	return m.Configs
}

// AssertValidConfiguration checks to ensure that the supplied 'MigConfig' is both valid and part of the 'MigConfigGroup'.
func (m *MigConfigGroupBase) AssertValidConfiguration(config MigConfig) error {
	err := config.AssertValid()
	if err != nil {
		return fmt.Errorf("invalid MigConfig: %v", err)
	}
	for _, c := range m.Configs {
		if config.IsSubsetOf(c) {
			return nil
		}
	}
	return fmt.Errorf("cannot configure as a subset of any valid configuration")
}
