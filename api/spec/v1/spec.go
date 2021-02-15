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
	"encoding/json"
	"fmt"

	"github.com/NVIDIA/mig-parted/pkg/types"
)

const Version = "v1"

type Spec struct {
	Version    string                        `json:"version"               yaml:"version"`
	MigConfigs map[string]MigConfigSpecSlice `json:"mig-configs,omitempty" yaml:"mig-configs,omitempty"`
}

type MigConfigSpec struct {
	DeviceFilter string          `json:"device-filter,omitempty" yaml:"device-filter,omitempty"`
	Devices      interface{}     `json:"devices"                 yaml:"devices,flow"`
	MigEnabled   bool            `json:"mig-enabled"             yaml:"mig-enabled"`
	MigDevices   types.MigConfig `json:"mig-devices"             yaml:"mig-devices"`
}

type MigConfigSpecSlice []MigConfigSpec

func containsKey(m map[string]json.RawMessage, s string) bool {
	_, exists := m[s]
	return exists
}

func (s *Spec) UnmarshalJSON(b []byte) error {
	spec := make(map[string]json.RawMessage)
	err := json.Unmarshal(b, &spec)
	if err != nil {
		return err
	}

	if !containsKey(spec, "version") && len(spec) > 0 {
		return fmt.Errorf("unable to parse with missing 'version' field")
	}

	result := Spec{}
	for k, v := range spec {
		switch k {
		case "version":
			var version string
			err := json.Unmarshal(v, &version)
			if err != nil {
				return err
			}
			result.Version = version
		}
	}

	if result.Version != Version {
		return fmt.Errorf("unknown version: %v", result.Version)
	}

	delete(spec, "version")
	for k, v := range spec {
		switch k {
		case "mig-configs":
			configs := map[string]MigConfigSpecSlice{}
			err := json.Unmarshal(v, &configs)
			if err != nil {
				return err
			}
			if len(configs) == 0 {
				return fmt.Errorf("at least one entry in '%v' is required", k)
			}
			for c, s := range configs {
				if len(s) == 0 {
					return fmt.Errorf("at least one entry in '%v' is required", c)
				}
			}
			result.MigConfigs = configs
		default:
			return fmt.Errorf("unexpected field: %v", k)
		}
	}

	*s = result
	return nil
}

func (s *MigConfigSpec) UnmarshalJSON(b []byte) error {
	spec := make(map[string]json.RawMessage)
	err := json.Unmarshal(b, &spec)
	if err != nil {
		return err
	}

	required := []string{"devices", "mig-enabled"}
	for _, r := range required {
		if !containsKey(spec, r) {
			return fmt.Errorf("missing required field: %v", r)
		}
	}

	result := MigConfigSpec{}
	for k, v := range spec {
		switch k {
		case "device-filter":
			var filter string
			err := json.Unmarshal(v, &filter)
			if err != nil {
				return err
			}
			result.DeviceFilter = filter
		case "devices":
			var str string
			err1 := json.Unmarshal(v, &str)
			if err1 == nil {
				if str != "all" {
					return fmt.Errorf("invalid string input for '%v': %v", k, str)
				}
				result.Devices = str
				break
			}
			var intslice []int
			err2 := json.Unmarshal(v, &intslice)
			if err2 == nil {
				result.Devices = intslice
				break
			}
			return fmt.Errorf("(%v, %v)", err1, err2)
		case "mig-enabled":
			var enabled bool
			err := json.Unmarshal(v, &enabled)
			if err != nil {
				return err
			}
			result.MigEnabled = enabled
		case "mig-devices":
			devices := make(types.MigConfig)
			err := json.Unmarshal(v, &devices)
			if err != nil {
				return err
			}
			err = devices.AssertValid()
			if err != nil {
				return fmt.Errorf("error validating values in '%v' field: %v", k, err)
			}
			result.MigDevices = devices
		default:
			return fmt.Errorf("unexpected field: %v", k)
		}
	}

	if result.MigEnabled && result.MigDevices == nil {
		return fmt.Errorf("missing required field 'mig-devices' when 'mig-enabled' is true")
	}

	if !result.MigEnabled && len(result.MigDevices) != 0 {
		return fmt.Errorf("MIG devices included when 'mig-enabled' is false")
	}

	*s = result
	return nil
}
