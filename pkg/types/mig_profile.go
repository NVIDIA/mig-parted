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
	nvdev "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
)

const (
	AttributeMediaExtensions = nvdev.AttributeMediaExtensions
)

// MigProfile represents a specific MIG profile.
// Examples include "1g.5gb", "2g.10gb", "1c.2g.10gb", or "1c.1g.5gb+me", etc.
type MigProfile struct {
	nvdev.MigProfileInfo
}

// NewMigProfile constructs a new Profile struct using info from the giProfiles and ciProfiles used to create it.
func NewMigProfile(giProfileID, ciProfileID, ciEngProfileID int, migMemorySizeMB, deviceMemorySizeBytes uint64) (*MigProfile, error) {
	mp, err := nvdevNewMigProfile(giProfileID, ciProfileID, ciEngProfileID, migMemorySizeMB, deviceMemorySizeBytes)
	if err != nil {
		return nil, err
	}
	return &MigProfile{mp.GetInfo()}, nil
}

// AssertValidMigProfileFormat checks if the string is in the proper format to represent a MIG profile
func AssertValidMigProfileFormat(profile string) error {
	return nvdevAssertValidMigProfileFormat(profile)
}

// ParseMigProfile converts a string representation of a MigProfile into an object.
func ParseMigProfile(profile string) (*MigProfile, error) {
	mp, err := nvdevParseMigProfile(profile)
	if err != nil {
		return nil, err
	}
	return &MigProfile{mp.GetInfo()}, nil
}

// MustParseMigProfile does the same as Parse(), but never throws an error.
func MustParseMigProfile(profile string) *MigProfile {
	m, _ := ParseMigProfile(profile)
	return m
}

// HasAttribute checks if the MigProfile has the specified attribute associated with it.
func (m MigProfile) HasAttribute(attr string) bool {
	for _, a := range m.Attributes {
		if a == attr {
			return true
		}
	}
	return false
}
