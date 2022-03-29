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

package mode

import (
	"fmt"
	"testing"

	"github.com/NVIDIA/mig-parted/internal/nvml"
	"github.com/stretchr/testify/require"
)

type Return = nvml.Return
type MockReturn = nvml.MockReturn

type mockNvmlA100Device struct {
	nvml.Device
	migCapable     bool
	driverBusy     bool
	currentMigMode int
	pendingMigMode int
}

func NewMockNvmlLunaServer() *nvmlMigModeManager {
	mls := &nvml.MockLunaServer{}
	for i := 0; i < 8; i++ {
		mls.Devices[i] = &mockNvmlA100Device{
			Device:         nvml.NewMockA100Device(i),
			migCapable:     true,
			driverBusy:     false,
			currentMigMode: nvml.DEVICE_MIG_DISABLE,
			pendingMigMode: nvml.DEVICE_MIG_DISABLE,
		}
	}
	return &nvmlMigModeManager{mls}
}

func (d *mockNvmlA100Device) SetMigMode(mode int) (Return, Return) {
	if !d.migCapable {
		return MockReturn(nvml.ERROR_NOT_SUPPORTED), MockReturn(nvml.ERROR_NOT_SUPPORTED)
	}

	d.pendingMigMode = mode
	if !d.driverBusy {
		d.currentMigMode = mode
		return MockReturn(nvml.ERROR_IN_USE), MockReturn(nvml.SUCCESS)
	}
	return MockReturn(nvml.SUCCESS), MockReturn(nvml.SUCCESS)
}

func (d *mockNvmlA100Device) GetMigMode() (int, int, Return) {
	if !d.migCapable {
		return -1, -1, MockReturn(nvml.ERROR_NOT_SUPPORTED)
	}
	return d.currentMigMode, d.pendingMigMode, MockReturn(nvml.SUCCESS)
}

func TestNvmlIsMigCapable(t *testing.T) {
	manager := NewMockNvmlLunaServer()

	numGPUs, ret := manager.nvml.DeviceGetCount()
	require.NotNil(t, ret, "Unexpected nil return from DeviceGetCount")
	require.Equal(t, ret.Value(), nvml.SUCCESS, "Unexpected return value from DeviceGetCount")

	for i := 0; i < numGPUs; i++ {
		t.Run(fmt.Sprintf("GPU %v", i), func(t *testing.T) {
			capable, err := manager.IsMigCapable(i)
			require.Nil(t, err, "Unexpected failure from IsMigCapable")
			require.True(t, capable)

			server := manager.nvml.(*nvml.MockLunaServer)
			device := server.Devices[i].(*mockNvmlA100Device)
			device.migCapable = false

			capable, err = manager.IsMigCapable(i)
			require.Nil(t, err, "Unexpected failure from IsMigCapable")
			require.False(t, capable)
		})
	}
}

func TestNvmlEnableDisableMig(t *testing.T) {
	manager := NewMockNvmlLunaServer()

	numGPUs, ret := manager.nvml.DeviceGetCount()
	require.NotNil(t, ret, "Unexpected nil return from DeviceGetCount")
	require.Equal(t, ret.Value(), nvml.SUCCESS, "Unexpected return value from DeviceGetCount")

	for i := 0; i < numGPUs; i++ {
		t.Run(fmt.Sprintf("GPU %v", i), func(t *testing.T) {
			err := manager.SetMigMode(i, Enabled)
			require.Nil(t, err, "Unexpected failure from SetMigMode")

			mode, err := manager.GetMigMode(i)
			require.Nil(t, err, "Unexpected failure from GetMigMode")
			require.Equal(t, Enabled, mode)

			err = manager.SetMigMode(i, Disabled)
			require.Nil(t, err, "Unexpected failure from SetMigMode")

			mode, err = manager.GetMigMode(i)
			require.Nil(t, err, "Unexpected failure from GetMigMode")
			require.Equal(t, Disabled, mode)

			server := manager.nvml.(*nvml.MockLunaServer)
			device := server.Devices[i].(*mockNvmlA100Device)
			device.driverBusy = true

			err = manager.SetMigMode(i, Enabled)
			require.Nil(t, err, "Unexpected failure from SetMigMode")

			mode, err = manager.GetMigMode(i)
			require.Nil(t, err, "Unexpected failure from GetMigMode")
			require.Equal(t, Disabled, mode)

			pending, err := manager.IsMigModeChangePending(i)
			require.Nil(t, err, "Unexpected failure from IsMigModeChangePending")
			require.True(t, pending)
		})
	}
}
