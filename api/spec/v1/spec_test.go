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
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/NVIDIA/mig-parted/pkg/types"
)

func TestMarshallUnmarshall(t *testing.T) {
	spec := Spec{
		Version: "v1",
		MigConfigs: map[string]MigConfigSpecSlice{
			"valid-format-non-existent-devices": []MigConfigSpec{
				{
					Devices:    "all",
					MigEnabled: true,
					MigDevices: types.MigConfig{
						"0g.0gb": 100,
					},
				},
			},
			"all-disabled": []MigConfigSpec{
				{
					DeviceFilter: "A100-SXM4-40GB",
					Devices:      "all",
					MigEnabled:   false,
				},
			},
			"all-1-slice": []MigConfigSpec{
				{
					DeviceFilter: "A100-SXM4-40GB",
					Devices:      "all",
					MigEnabled:   true,
					MigDevices: types.MigConfig{
						"1g.5gb": 7,
					},
				},
			},
			"all-2-slice": []MigConfigSpec{
				{
					DeviceFilter: "A100-SXM4-40GB",
					Devices:      "all",
					MigEnabled:   true,
					MigDevices: types.MigConfig{
						"2g.10gb": 3,
					},
				},
			},
			"all-3-slice": []MigConfigSpec{
				{
					DeviceFilter: "A100-SXM4-40GB",
					Devices:      "all",
					MigEnabled:   true,
					MigDevices: types.MigConfig{
						"3g.20gb": 2,
					},
				},
			},
			"all-balanced-slices": []MigConfigSpec{
				{
					DeviceFilter: "A100-SXM4-40GB",
					Devices:      "all",
					MigEnabled:   true,
					MigDevices: types.MigConfig{
						"1g.5gb":  2,
						"2g.10gb": 1,
						"3g.20gb": 1,
					},
				},
			},
			"half-disabled-half-balanced-slices": []MigConfigSpec{
				{
					DeviceFilter: "A100-SXM4-40GB",
					Devices:      []int{0, 1, 2, 3},
					MigEnabled:   false,
				},
				{
					DeviceFilter: "A100-SXM4-40GB",
					Devices:      []int{4, 5, 6, 7},
					MigEnabled:   true,
					MigDevices: types.MigConfig{
						"1g.5gb":  2,
						"2g.10gb": 1,
						"3g.20gb": 1,
					},
				},
			},
			"multi-device-filter": []MigConfigSpec{
				{
					DeviceFilter: []string{"A100-SXM4-40GB", "A100-PCIE-40GB"},
					Devices:      []int{0, 1, 2, 3},
					MigEnabled:   false,
				},
				{
					DeviceFilter: []string{"A100-SXM4-40GB", "A100-PCIE-40GB"},
					Devices:      []int{4, 5, 6, 7},
					MigEnabled:   true,
					MigDevices: types.MigConfig{
						"1g.5gb":  2,
						"2g.10gb": 1,
						"3g.20gb": 1,
					},
				},
			},
		},
	}

	types.SetMockNVdevlib()
	y, err := yaml.Marshal(spec)
	require.Nil(t, err, "Unexpected failure yaml.Marshal")

	s := Spec{}
	err = yaml.Unmarshal(y, &s)
	require.Nil(t, err, "Unexpected failure yaml.Unmarshal")
	require.Equal(t, spec, s)
}

func TestSpec(t *testing.T) {
	testCases := []struct {
		Description     string
		Spec            string
		expectedFailure bool
	}{
		{
			"Empty",
			"",
			false,
		},
		{
			"Well formed",
			`{
				"version": "v1",
				"mig-configs": {
					"all-disabled": [{
						"devices": "all",
						"mig-enabled": false
					}]
				}
			}`,
			false,
		},
		{
			"Only version field",
			`{
				"version": "v1"
			}`,
			false,
		},
		{
			"Well formed - wrong version",
			`{
				"version": "v2",
				"mig-configs": {
					"all-disabled": [{
						"devices": "all",
						"mig-enabled": false
					}]
				}
			}`,
			true,
		},
		{
			"Only version field - wrong versiomn",
			`{
				"version": "v2"
			}`,
			true,
		},
		{
			"Missing version field",
			`{
				"mig-configs": {
					"all-disabled": [{
						"devices": "all",
						"mig-enabled": false
					}]
				}
			}`,
			true,
		},
		{
			"Erroneous field",
			`{
				"bogus": "field",
				"version": "v1",
				"mig-configs": {
					"all-disabled": [{
						"devices": "all",
						"mig-enabled": false
					}]
				}
			}`,
			true,
		},
		{
			"Empty 'mig-configs'",
			`{
				"version": "v1",
				"mig-configs": {}
			}`,
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			s := Spec{}
			err := yaml.Unmarshal([]byte(tc.Spec), &s)
			if tc.expectedFailure {
				require.NotNil(t, err, "Unexpected success yaml.Unmarshal")
			} else {
				require.Nil(t, err, "Unexpected failure yaml.Unmarshal")
			}
		})
	}
}

func TestMigConfigSpec(t *testing.T) {
	testCases := []struct {
		Description     string
		Spec            string
		expectedFailure bool
	}{
		{
			"Empty",
			"",
			false,
		},
		{
			"Well formed",
			`{
				"devices": "all",
				"mig-enabled": true,
				"mig-devices": {
					"1g.5gb": 2
				}
			}`,
			false,
		},
		{
			"Well formed with filter",
			`{
				"device-filter": "MODEL",
				"devices": "all",
				"mig-enabled": true,
				"mig-devices": {
					"1g.5gb": 2
				}
			}`,
			false,
		},
		{
			"Well formed with multi-filter",
			`{
				"device-filter": ["MODEL1", "MODEL2"],
				"devices": "all",
				"mig-enabled": true,
				"mig-devices": {
					"1g.5gb": 2
				}
			}`,
			false,
		},
		{
			"Missing 'devices'",
			`{
				"mig-enabled": true,
				"mig-devices": {
					"1g.5gb": 2
				}
			}`,
			true,
		},
		{
			"Missing 'mig-enabled'",
			`{
				"devices": "all",
				"mig-devices": {
					"1g.5gb": 2
				}
			}`,
			true,
		},
		{
			"Missing 'mig-devices', enabled: false",
			`{
				"devices": "all",
				"mig-enabled": false,
			}`,
			false,
		},
		{
			"Missing 'mig-devices', enabled: true",
			`{
				"devices": "all",
				"mig-enabled": true,
			}`,
			true,
		},
		{
			"'mig-devices' formatted correctly",
			`{
				"devices": "all",
				"mig-enabled": true,
				"mig-devices": {
					"1g.5gb": 2
				}
			}`,
			false,
		},
		{
			"'mig-devices' formatted incorrectly",
			`{
				"devices": "all",
				"mig-enabled": true,
				"mig-devices": {
					"bogus": 2
				}
			}`,
			true,
		},
		{
			"Erroneous field",
			`{
				"bogus": "field",
				"devices": "all",
				"mig-enabled": false,
			}`,
			true,
		},
		{
			"'devices' string == all",
			`{
				"devices": "all",
				"mig-enabled": false,
			}`,
			false,
		},
		{
			"'devices' string != all",
			`{
				"devices": "bogus",
				"mig-enabled": false,
			}`,
			true,
		},
		{
			"'devices' []int",
			`{
				"devices": [0,1,2,3],
				"mig-enabled": false,
			}`,
			false,
		},
		{
			"'devices' not string for []int",
			`{
				"devices": {},
				"mig-enabled": false,
			}`,
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			var s MigConfigSpec
			err := yaml.Unmarshal([]byte(tc.Spec), &s)
			if tc.expectedFailure {
				require.NotNil(t, err, "Unexpected success yaml.Unmarshal")
			} else {
				require.Nil(t, err, "Unexpected failure yaml.Unmarshal")
			}
		})
	}
}
