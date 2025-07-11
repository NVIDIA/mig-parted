/**
# SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package reconfigure

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

type commandRunnerWithCLI struct {
	mock  *commandRunnerMock
	calls [][]string
}

func (c *commandRunnerWithCLI) Run(cmd *exec.Cmd) error {
	c.calls = append(c.calls, append([]string{cmd.Path}, cmd.Args...))
	return c.mock.Run(cmd)
}

func TestReconfigure(t *testing.T) {
	testCases := []struct {
		description    string
		options        reconfigureMIGOptions
		commandRunner  *commandRunnerWithCLI
		migParted      *migPartedMock
		checkMigParted func(*migPartedMock)
		expectedError  error
		expectedCalls  [][]string
	}{
		{
			description: "mig assert valid config failure does not call commands",
			options: reconfigureMIGOptions{
				NodeName:            "NodeName",
				MIGPartedConfigFile: "/path/to/config/file.yaml",
				SelectedMIGConfig:   "selected-mig-config",
				DriverLibraryPath:   "/path/to/libnvidia-ml.so.1",
				HostRootMount:       "/host/",
			},
			commandRunner: &commandRunnerWithCLI{
				mock: &commandRunnerMock{
					RunFunc: func(cmd *exec.Cmd) error {
						return fmt.Errorf("error running command %v", cmd.Path)
					},
				},
			},
			migParted: &migPartedMock{
				assertValidMIGConfigFunc: func() error {
					return fmt.Errorf("invalid mig config")
				},
			},
			checkMigParted: func(mpm *migPartedMock) {
				require.Len(t, mpm.calls.assertValidMIGConfig, 1)
				require.Len(t, mpm.calls.applyMIGConfig, 0)
				require.Len(t, mpm.calls.assertMIGModeOnly, 0)
				require.Len(t, mpm.calls.applyMIGModeOnly, 0)
				require.Len(t, mpm.calls.applyMIGConfig, 0)
			},
			expectedError: fmt.Errorf("error validating the selected MIG configuration: invalid mig config"),
			expectedCalls: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			// TODO: Once we have better mocks in place for the following
			// functionality, we can update this.
			require.False(t, tc.options.WithReboot)
			require.False(t, tc.options.WithShutdownHostGPUClients)

			// We test explicit validation in a separate test.
			// For now we only ensure that the options are valid.
			require.NoError(t, tc.options.Validate())

			r := &reconfigurer{
				reconfigureMIGOptions: &tc.options,
				commandRunner:         tc.commandRunner,
				migParted:             tc.migParted,
			}

			err := r.Reconfigure()
			require.EqualValues(t, tc.expectedError.Error(), err.Error())

			tc.checkMigParted(tc.migParted)

			require.EqualValues(t, tc.expectedCalls, tc.commandRunner.calls)

		})
	}
}
