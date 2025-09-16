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

type nodeWithLabels struct {
	mock      *nodeLabellerMock
	setLabels map[string]string
}

func (n *nodeWithLabels) getNodeLabelValue(label string) (string, error) {
	return n.mock.getNodeLabelValue(label)
}

func (n *nodeWithLabels) setNodeLabelValue(label string, value string) error {
	if err := n.mock.setNodeLabelValue(label, value); err != nil {
		return err
	}
	if n.setLabels == nil {
		n.setLabels = make(map[string]string)
	}
	n.setLabels[label] = value
	return nil
}

func TestReconfigure(t *testing.T) {
	testCases := []struct {
		description       string
		options           reconfigureMIGOptions
		migParted         *migPartedMock
		checkMigParted    func(*migPartedMock)
		nodeLabeller      *nodeWithLabels
		checkNodeLabeller func(*nodeWithLabels)
		expectedError     error
		expectedCalls     [][]string
	}{
		{
			description: "mig assert valid config failure does not call commands",
			options: reconfigureMIGOptions{
				NodeName:            "NodeName",
				MIGPartedConfigFile: "/path/to/config/file.yaml",
				SelectedMIGConfig:   "selected-mig-config",
				DriverLibraryPath:   "/path/to/libnvidia-ml.so.1",
				HostRootMount:       "/host/",
				ConfigStateLabel:    "example.com/config.state",
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
		{
			description: "node label error is causes exit",
			options: reconfigureMIGOptions{
				NodeName:            "NodeName",
				MIGPartedConfigFile: "/path/to/config/file.yaml",
				SelectedMIGConfig:   "selected-mig-config",
				DriverLibraryPath:   "/path/to/libnvidia-ml.so.1",
				HostRootMount:       "/host/",
				ConfigStateLabel:    "example.com/config.state",
			},
			migParted: &migPartedMock{
				assertValidMIGConfigFunc: func() error {
					return nil
				},
			},
			checkMigParted: func(mpm *migPartedMock) {
				require.Len(t, mpm.calls.assertValidMIGConfig, 1)
				require.Len(t, mpm.calls.applyMIGConfig, 0)
				require.Len(t, mpm.calls.assertMIGModeOnly, 0)
				require.Len(t, mpm.calls.applyMIGModeOnly, 0)
				require.Len(t, mpm.calls.applyMIGConfig, 0)
			},
			nodeLabeller: &nodeWithLabels{
				mock: &nodeLabellerMock{
					getNodeLabelValueFunc: func(s string) (string, error) {
						return "", fmt.Errorf("error getting label")
					},
				},
			},
			checkNodeLabeller: func(nwl *nodeWithLabels) {
				calls := nwl.mock.getNodeLabelValueCalls()
				require.Len(t, calls, 1)
				require.EqualValues(t, []struct{ S string }{{"example.com/config.state"}}, calls)
			},
			expectedError: fmt.Errorf(`unable to get the value of the "example.com/config.state" label: error getting label`),
		},
		{
			description: "reconfigure exits if config is applied",
			options: reconfigureMIGOptions{
				NodeName:            "NodeName",
				MIGPartedConfigFile: "/path/to/config/file.yaml",
				SelectedMIGConfig:   "selected-mig-config",
				DriverLibraryPath:   "/path/to/libnvidia-ml.so.1",
				HostRootMount:       "/host/",
				ConfigStateLabel:    "example.com/config.state",
			},
			migParted: &migPartedMock{
				assertValidMIGConfigFunc: func() error {
					return nil
				},
				assertMIGConfigFunc: func() error {
					return nil
				},
			},
			checkMigParted: func(mpm *migPartedMock) {
				require.Len(t, mpm.calls.assertValidMIGConfig, 1)
				require.Len(t, mpm.calls.assertMIGConfig, 1)
				require.Len(t, mpm.calls.applyMIGConfig, 0)
				require.Len(t, mpm.calls.assertMIGModeOnly, 0)
				require.Len(t, mpm.calls.applyMIGModeOnly, 0)
			},
			nodeLabeller: &nodeWithLabels{
				mock: &nodeLabellerMock{
					getNodeLabelValueFunc: func(s string) (string, error) {
						return "current-state", nil
					},
				},
			},
			checkNodeLabeller: func(nwl *nodeWithLabels) {
				calls := nwl.mock.getNodeLabelValueCalls()
				require.Len(t, calls, 1)
				require.EqualValues(t, []struct{ S string }{{"example.com/config.state"}}, calls)
			},
			expectedError: nil,
		},
		{
			description: "mode change required after reboot is error",
			options: reconfigureMIGOptions{
				NodeName:            "NodeName",
				MIGPartedConfigFile: "/path/to/config/file.yaml",
				SelectedMIGConfig:   "selected-mig-config",
				DriverLibraryPath:   "/path/to/libnvidia-ml.so.1",
				HostRootMount:       "/host/",
				ConfigStateLabel:    "example.com/config.state",
			},
			migParted: &migPartedMock{
				assertValidMIGConfigFunc: func() error {
					return nil
				},
				assertMIGConfigFunc: func() error {
					return fmt.Errorf("config needs updating")
				},
				assertMIGModeOnlyFunc: func() error {
					return fmt.Errorf("mode needs updating")
				},
			},
			checkMigParted: func(mpm *migPartedMock) {
				require.Len(t, mpm.calls.assertValidMIGConfig, 1)
				require.Len(t, mpm.calls.assertMIGConfig, 1)
				require.Len(t, mpm.calls.assertMIGModeOnly, 1)
				require.Len(t, mpm.calls.applyMIGConfig, 0)
				require.Len(t, mpm.calls.applyMIGModeOnly, 0)
			},
			nodeLabeller: &nodeWithLabels{
				mock: &nodeLabellerMock{
					getNodeLabelValueFunc: func(s string) (string, error) {
						return "rebooting", nil
					},
				},
			},
			checkNodeLabeller: func(nwl *nodeWithLabels) {
				calls := nwl.mock.getNodeLabelValueCalls()
				require.Len(t, calls, 1)
				require.EqualValues(t, []struct{ S string }{{"example.com/config.state"}}, calls)
			},
			expectedError: fmt.Errorf("MIG mode change failed after reboot: mode needs updating"),
		},
		{
			description: "mode does not need updating; apply config error is returned",
			options: reconfigureMIGOptions{
				NodeName:            "NodeName",
				MIGPartedConfigFile: "/path/to/config/file.yaml",
				SelectedMIGConfig:   "selected-mig-config",
				DriverLibraryPath:   "/path/to/libnvidia-ml.so.1",
				HostRootMount:       "/host/",
				ConfigStateLabel:    "example.com/config.state",
			},
			migParted: &migPartedMock{
				assertValidMIGConfigFunc: func() error {
					return nil
				},
				assertMIGConfigFunc: func() error {
					return fmt.Errorf("config needs updating")
				},
				assertMIGModeOnlyFunc: func() error {
					return nil
				},
				applyMIGModeOnlyFunc: func() error {
					return nil
				},
				applyMIGConfigFunc: func() error {
					return fmt.Errorf("failed to apply config")
				},
			},
			checkMigParted: func(mpm *migPartedMock) {
				require.Len(t, mpm.calls.assertValidMIGConfig, 1)
				require.Len(t, mpm.calls.assertMIGConfig, 1)
				require.Len(t, mpm.calls.assertMIGModeOnly, 2)
				require.Len(t, mpm.calls.applyMIGModeOnly, 1)
				require.Len(t, mpm.calls.applyMIGConfig, 1)
			},
			nodeLabeller: &nodeWithLabels{
				mock: &nodeLabellerMock{
					getNodeLabelValueFunc: func(s string) (string, error) {
						return "current-state", nil
					},
				},
			},
			checkNodeLabeller: func(nwl *nodeWithLabels) {
				calls := nwl.mock.getNodeLabelValueCalls()
				require.Len(t, calls, 1)
				require.EqualValues(t, []struct{ S string }{{"example.com/config.state"}}, calls)
			},
			expectedError: fmt.Errorf("failed to apply config"),
		},
		{
			description: "mode does not need updating; apply config succeeds",
			options: reconfigureMIGOptions{
				NodeName:            "NodeName",
				MIGPartedConfigFile: "/path/to/config/file.yaml",
				SelectedMIGConfig:   "selected-mig-config",
				DriverLibraryPath:   "/path/to/libnvidia-ml.so.1",
				HostRootMount:       "/host/",
				ConfigStateLabel:    "example.com/config.state",
			},
			migParted: &migPartedMock{
				assertValidMIGConfigFunc: func() error {
					return nil
				},
				assertMIGConfigFunc: func() error {
					return fmt.Errorf("config needs updating")
				},
				assertMIGModeOnlyFunc: func() error {
					return nil
				},
				applyMIGModeOnlyFunc: func() error {
					return nil
				},
				applyMIGConfigFunc: func() error {
					return nil
				},
			},
			checkMigParted: func(mpm *migPartedMock) {
				require.Len(t, mpm.calls.assertValidMIGConfig, 1)
				require.Len(t, mpm.calls.assertMIGConfig, 1)
				require.Len(t, mpm.calls.assertMIGModeOnly, 2)
				require.Len(t, mpm.calls.applyMIGModeOnly, 1)
				require.Len(t, mpm.calls.applyMIGConfig, 1)
			},
			nodeLabeller: &nodeWithLabels{
				mock: &nodeLabellerMock{
					getNodeLabelValueFunc: func(s string) (string, error) {
						return "current-state", nil
					},
				},
			},
			checkNodeLabeller: func(nwl *nodeWithLabels) {
				calls := nwl.mock.getNodeLabelValueCalls()
				require.Len(t, calls, 1)
				require.EqualValues(t, []struct{ S string }{{"example.com/config.state"}}, calls)
			},
			expectedError: nil,
		},
	}

	for _, tc := range testCases {
		commandRunner := &commandRunnerWithCLI{
			mock: &commandRunnerMock{
				RunFunc: func(cmd *exec.Cmd) error {
					return fmt.Errorf("error running command %v", cmd.Path)
				},
			},
		}

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
				commandRunner:         commandRunner,
				migParted:             tc.migParted,
				node:                  tc.nodeLabeller,
			}

			err := r.Reconfigure()
			if tc.expectedError == nil {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.expectedError.Error())
			}

			if tc.checkMigParted != nil {
				tc.checkMigParted(tc.migParted)
			}
			if tc.checkNodeLabeller != nil {
				tc.checkNodeLabeller(tc.nodeLabeller)
			}

			require.EqualValues(t, tc.expectedCalls, commandRunner.calls)
		})
	}
}
