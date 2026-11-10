/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package restore

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
	"github.com/NVIDIA/mig-parted/pkg/mig/state"
	"github.com/NVIDIA/mig-parted/pkg/types"
)

// fetchOnlyStateManager serves a fixed node state to AssertMigMode. Restoring
// is out of scope here, so both restore calls are left unimplemented.
type fetchOnlyStateManager struct {
	state *types.MigState
	err   error
}

var _ state.Manager = (*fetchOnlyStateManager)(nil)

func (m *fetchOnlyStateManager) Fetch() (*types.MigState, error) {
	return m.state, m.err
}

func (m *fetchOnlyStateManager) RestoreMode(*types.MigState) error {
	panic("not implemented")
}

func (m *fetchOnlyStateManager) RestoreConfig(*types.MigState) error {
	panic("not implemented")
}

func TestAssertMigMode(t *testing.T) {
	testCases := []struct {
		description string
		node        []types.DeviceState
		checkpoint  []types.DeviceState
		fetchErr    error
		errContains string
	}{
		{
			description: "matching mode on every device",
			node: []types.DeviceState{
				{UUID: "GPU-0", MigMode: mode.Enabled},
				{UUID: "GPU-1", MigMode: mode.Disabled},
			},
			checkpoint: []types.DeviceState{
				{UUID: "GPU-0", MigMode: mode.Enabled},
				{UUID: "GPU-1", MigMode: mode.Disabled},
			},
		},
		{
			description: "differing mode on a device",
			node: []types.DeviceState{
				{UUID: "GPU-0", MigMode: mode.Disabled},
			},
			checkpoint: []types.DeviceState{
				{UUID: "GPU-0", MigMode: mode.Enabled},
			},
			errContains: "current mode different than mode being asserted",
		},
		{
			description: "checkpoint holds more devices than the node",
			node: []types.DeviceState{
				{UUID: "GPU-0", MigMode: mode.Enabled},
			},
			checkpoint: []types.DeviceState{
				{UUID: "GPU-0", MigMode: mode.Enabled},
				{UUID: "GPU-1", MigMode: mode.Enabled},
			},
			errContains: "GPU-1",
		},
		{
			description: "node holds a device the checkpoint does not",
			node: []types.DeviceState{
				{UUID: "GPU-0", MigMode: mode.Disabled},
				{UUID: "GPU-1", MigMode: mode.Enabled},
			},
			checkpoint: []types.DeviceState{
				{UUID: "GPU-1", MigMode: mode.Enabled},
			},
		},
		{
			description: "same devices in a different order",
			node: []types.DeviceState{
				{UUID: "GPU-0", MigMode: mode.Enabled},
				{UUID: "GPU-1", MigMode: mode.Disabled},
			},
			checkpoint: []types.DeviceState{
				{UUID: "GPU-1", MigMode: mode.Disabled},
				{UUID: "GPU-0", MigMode: mode.Enabled},
			},
		},
		{
			description: "checkpoint names a device that is not on the node",
			node: []types.DeviceState{
				{UUID: "GPU-0", MigMode: mode.Enabled},
			},
			checkpoint: []types.DeviceState{
				{UUID: "GPU-7", MigMode: mode.Enabled},
			},
			errContains: "GPU-7",
		},
		{
			description: "the node state cannot be fetched",
			checkpoint: []types.DeviceState{
				{UUID: "GPU-0", MigMode: mode.Enabled},
			},
			fetchErr:    fmt.Errorf("NVML is not loaded"),
			errContains: "NVML is not loaded",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			manager := &fetchOnlyStateManager{
				state: &types.MigState{Devices: tc.node},
				err:   tc.fetchErr,
			}
			c := &Context{
				MigState:        &types.MigState{Devices: tc.checkpoint},
				MigStateManager: manager,
			}

			err := c.AssertMigMode()
			if tc.errContains == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.errContains)
		})
	}
}
