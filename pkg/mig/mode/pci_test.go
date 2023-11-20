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

	"github.com/stretchr/testify/require"

	"github.com/NVIDIA/go-nvlib/pkg/nvpci"
)

type mockPciMigModeManager struct {
	*pciMigModeManager
	driverBusy bool
}

func NewMockPciA100Device() (*mockPciMigModeManager, error) {
	nvpci, err := nvpci.NewMockNvpci()
	if err != nil {
		return nil, fmt.Errorf("error creating Mock A100 PCI device: %v", err)
	}

	err = nvpci.AddMockA100("0000:80:05.1", 0)
	if err != nil {
		return nil, fmt.Errorf("error adding Mock A100 device to MockNvpci: %v", err)
	}

	mock := &mockPciMigModeManager{
		&pciMigModeManager{nvpci},
		false,
	}

	return mock, nil
}

func (m *mockPciMigModeManager) Cleanup() {
	m.nvpci.(*nvpci.MockNvpci).Cleanup()
}

func (m *mockPciMigModeManager) SetBooted(gpu int, booted bool) error {
	bar0, err := m.openBar0(0)
	if err != nil {
		return err
	}
	defer bar0.Close()
	if booted {
		bar0.Write32(BootCompleteReg, BootCompleteValue)
	} else {
		bar0.Write32(BootCompleteReg, 0)
	}
	return nil
}

func (m *mockPciMigModeManager) SetMigCapable(gpu int, capable bool) error {
	bar0, err := m.openBar0(0)
	if err != nil {
		return err
	}
	defer bar0.Close()
	if capable {
		bar0.Write32(PmcIDReg, migCapablePmcIDs[0])
	} else {
		bar0.Write32(PmcIDReg, 0xdeadbeef)
	}
	return nil
}

func (m *mockPciMigModeManager) SetMigMode(gpu int, mode MigMode) error {
	err := m.pciMigModeManager.SetMigMode(gpu, mode)
	if err != nil {
		return err
	}

	bar0, err := m.openBar0(0)
	if err != nil {
		return err
	}
	defer bar0.Close()

	if !m.driverBusy {
		if mode == Enabled {
			m.setBitsInReg(bar0, MigModeCheckReg, MigModeCheckEnabled)
		} else {
			m.clearBitsInReg(bar0, MigModeCheckReg, MigModeCheckEnabled)
		}
	}

	return nil
}

func TestPciIsMigCapable(t *testing.T) {
	manager, err := NewMockPciA100Device()
	require.Nil(t, err, "Error creating MockPciA100Device")
	defer manager.Cleanup()

	err = manager.SetBooted(0, true)
	require.Nil(t, err, "Unexpected failure from SetBooted")

	capable, err := manager.IsMigCapable(0)
	require.Nil(t, err, "Unexpected failure from IsMigCapable")
	require.True(t, capable)

	err = manager.SetMigCapable(0, false)
	require.Nil(t, err, "Unexpected failure from SetMigCapable")

	capable, err = manager.IsMigCapable(0)
	require.Nil(t, err, "Unexpected failure from IsMigCapable")
	require.False(t, capable)
}

func TestPciEnableDisableMig(t *testing.T) {
	manager, err := NewMockPciA100Device()
	require.Nil(t, err, "Error creating MockPciA100Device")
	defer manager.Cleanup()

	err = manager.SetBooted(0, true)
	require.Nil(t, err, "Unexpected failure from SetBooted")

	err = manager.SetMigMode(0, Enabled)
	require.Nil(t, err, "Unexpected failure from SetMigMode")

	mode, err := manager.GetMigMode(0)
	require.Nil(t, err, "Unexpected failure from GetMigMode")
	require.Equal(t, Enabled, mode)

	err = manager.SetMigMode(0, Disabled)
	require.Nil(t, err, "Unexpected failure from SetMigMode")

	mode, err = manager.GetMigMode(0)
	require.Nil(t, err, "Unexpected failure from GetMigMode")
	require.Equal(t, Disabled, mode)

	manager.driverBusy = true

	err = manager.SetMigMode(0, Enabled)
	require.Nil(t, err, "Unexpected failure from SetMigMode")

	mode, err = manager.GetMigMode(0)
	require.Nil(t, err, "Unexpected failure from GetMigMode")
	require.Equal(t, Disabled, mode)

	pending, err := manager.IsMigModeChangePending(0)
	require.Nil(t, err, "Unexpected failure from IsMigModeChangePending")
	require.True(t, pending)
}
