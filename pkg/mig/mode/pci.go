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
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/NVIDIA/go-nvlib/pkg/nvpci"
	"github.com/NVIDIA/go-nvlib/pkg/nvpci/mmio"
)

const (
	PmcIDReg = 0

	BootCompleteReg   = 0x118234
	BootCompleteValue = uint32(0x03FF)

	MigModeCheckReg     = 0x1404
	MigModeCheckEnabled = uint32(0x8000) // Bits 13:15 100

	MigModeSetReg      = 0x118F78
	MigModeSetMask     = uint32(0xC000) // Bits 14:15 11
	MigModeSetEnabled  = uint32(0xC000) // Bits 14:15 11
	MigModeSetDisabled = uint32(0x8000) // Bits 14:15 10

	WaitForBootTimeout       = 5 * time.Second
	WaitForBootSleepInterval = 100 * time.Millisecond
)

var migCapablePmcIDs = []uint32{
	0x170000a1, // GA100 Chip Set
}

type pciMigModeManager struct {
	nvpci nvpci.Interface
}

var _ Manager = (*pciMigModeManager)(nil)

func NewPciMigModeManager() Manager {
	return &pciMigModeManager{nvpci.New()}
}

func (m *pciMigModeManager) openBar0(gpu int) (mmio.Mmio, error) {
	gpus, err := m.nvpci.GetGPUs()
	if err != nil {
		return nil, fmt.Errorf("error getting list of GPUs: %v", err)
	}

	if gpu >= len(gpus) {
		return nil, fmt.Errorf("GPU index out of range: %v", gpu)
	}

	device := gpus[gpu]
	if len(device.Resources) < 1 {
		return nil, fmt.Errorf("missing bar0 MMIO resource")
	}

	bar0, err := device.Resources[0].OpenRW()
	if err != nil {
		return nil, fmt.Errorf("error opening bar0 MMIO resource: %v", err)
	}

	return bar0, nil
}

func (m *pciMigModeManager) openBar0ReadOnly(gpu int) (mmio.Mmio, error) {
	gpus, err := m.nvpci.GetGPUs()
	if err != nil {
		return nil, fmt.Errorf("error getting list of GPUs: %v", err)
	}

	if gpu >= len(gpus) {
		return nil, fmt.Errorf("GPU index out of range: %v", gpu)
	}

	device := gpus[gpu]
	if len(device.Resources) < 1 {
		return nil, fmt.Errorf("missing bar0 MMIO resource")
	}

	bar0, err := device.Resources[0].OpenRO()
	if err != nil {
		return nil, fmt.Errorf("error opening bar0 MMIO resource: %v", err)
	}

	return bar0, nil
}

func (m *pciMigModeManager) tryCloseBar0(bar0 mmio.Mmio) {
	err := bar0.Close()
	if err != nil {
		log.Warnf("error closing bar0 MMIO resource: %v", err)
	}
}

func (m *pciMigModeManager) waitForBoot(bar0 mmio.Mmio) error {
	const iterations = int(WaitForBootTimeout / WaitForBootSleepInterval)
	attempts := 0
	for {
		reg := bar0.Read32(BootCompleteReg)
		if reg == BootCompleteValue {
			return nil
		}
		attempts++
		if attempts == iterations {
			break
		}
		time.Sleep(WaitForBootSleepInterval)
	}
	return fmt.Errorf("timeout after %v seconds", WaitForBootTimeout)
}

func (m *pciMigModeManager) openBar0AndWaitForBoot(gpu int) (_ mmio.Mmio, rerr error) {
	bar0, err := m.openBar0(gpu)
	if err != nil {
		return nil, err
	}
	defer func() {
		if rerr != nil {
			m.tryCloseBar0(bar0)
		}
	}()

	err = m.waitForBoot(bar0)
	if err != nil {
		return nil, fmt.Errorf("error waiting for GPU to boot: %v", err)
	}

	return bar0, nil
}

func (m *pciMigModeManager) openBar0ReadOnlyAndWaitForBoot(gpu int) (_ mmio.Mmio, rerr error) {
	bar0, err := m.openBar0ReadOnly(gpu)
	if err != nil {
		return nil, err
	}
	defer func() {
		if rerr != nil {
			m.tryCloseBar0(bar0)
		}
	}()

	err = m.waitForBoot(bar0)
	if err != nil {
		return nil, fmt.Errorf("error waiting for GPU to boot: %v", err)
	}

	return bar0, nil
}

func (m *pciMigModeManager) checkBitsInRegWithMask(bar0 mmio.Mmio, reg int, mask, bits uint32) bool {
	return (bar0.Read32(reg) & mask) == bits
}

func (m *pciMigModeManager) writeBitsInRegWithMask(bar0 mmio.Mmio, reg int, mask, bits uint32) {
	current := bar0.Read32(reg)
	masked := current & ^mask
	updated := masked | (bits & mask)
	bar0.Write32(reg, updated)
}

func (m *pciMigModeManager) checkBitsInReg(bar0 mmio.Mmio, reg int, bits uint32) bool {
	return m.checkBitsInRegWithMask(bar0, reg, bits, bits)
}

func (m *pciMigModeManager) setBitsInReg(bar0 mmio.Mmio, reg int, bits uint32) {
	m.writeBitsInRegWithMask(bar0, reg, bits, bits)
}

func (m *pciMigModeManager) clearBitsInReg(bar0 mmio.Mmio, reg int, bits uint32) {
	m.writeBitsInRegWithMask(bar0, reg, bits, ^bits)
}

func (m *pciMigModeManager) isMigCapable(bar0 mmio.Mmio) bool {
	pmcID := bar0.Read32(PmcIDReg)
	for _, id := range migCapablePmcIDs {
		if pmcID == id {
			return true
		}
	}
	return false
}

func (m *pciMigModeManager) isMigModeEnabled(bar0 mmio.Mmio) bool {
	return m.checkBitsInReg(bar0, MigModeCheckReg, MigModeCheckEnabled)
}

func (m *pciMigModeManager) setMigModeEnabled(bar0 mmio.Mmio) {
	m.writeBitsInRegWithMask(bar0, MigModeSetReg, MigModeSetMask, MigModeSetEnabled)
}

func (m *pciMigModeManager) setMigModeDisabled(bar0 mmio.Mmio) {
	m.writeBitsInRegWithMask(bar0, MigModeSetReg, MigModeSetMask, MigModeSetDisabled)
}

func (m *pciMigModeManager) isMigModeChangePending(bar0 mmio.Mmio) bool {
	enabled := m.isMigModeEnabled(bar0)
	pendingEnable := m.checkBitsInRegWithMask(bar0, MigModeSetReg, MigModeSetMask, MigModeSetEnabled)
	pendingDisable := m.checkBitsInRegWithMask(bar0, MigModeSetReg, MigModeSetMask, MigModeSetDisabled)
	if enabled && pendingDisable {
		return true
	}
	if !enabled && pendingEnable {
		return true
	}
	return false
}

func (m *pciMigModeManager) IsMigCapable(gpu int) (bool, error) {
	bar0, err := m.openBar0ReadOnly(gpu)
	if err != nil {
		return false, err
	}
	defer m.tryCloseBar0(bar0)
	return m.isMigCapable(bar0), nil
}

func (m *pciMigModeManager) GetMigMode(gpu int) (MigMode, error) {
	bar0, err := m.openBar0ReadOnlyAndWaitForBoot(gpu)
	if err != nil {
		return -1, err
	}
	defer m.tryCloseBar0(bar0)

	if !m.isMigCapable(bar0) {
		return -1, fmt.Errorf("non Mig-capable GPU")
	}

	if m.isMigModeEnabled(bar0) {
		return Enabled, nil
	}
	return Disabled, nil
}

func (m *pciMigModeManager) SetMigMode(gpu int, mode MigMode) error {
	capable, err := m.IsMigCapable(gpu)
	if err != nil {
		return fmt.Errorf("error checking if GPU is MIG capable: %v", err)
	}

	if !capable {
		return fmt.Errorf("non Mig-capable GPU")
	}

	bar0, err := m.openBar0AndWaitForBoot(gpu)
	if err != nil {
		return err
	}
	defer m.tryCloseBar0(bar0)

	switch mode {
	case Disabled:
		m.setMigModeDisabled(bar0)
	case Enabled:
		m.setMigModeEnabled(bar0)
	default:
		return fmt.Errorf("unknown Mig mode selected: %v", mode)
	}

	err = bar0.Sync()
	if err != nil {
		return fmt.Errorf("error syncing writes to bar0: %v", err)
	}

	return nil
}

func (m *pciMigModeManager) IsMigModeChangePending(gpu int) (bool, error) {
	bar0, err := m.openBar0ReadOnlyAndWaitForBoot(gpu)
	if err != nil {
		return false, err
	}
	defer m.tryCloseBar0(bar0)

	if !m.isMigCapable(bar0) {
		return false, fmt.Errorf("non Mig-capable GPU")
	}

	return m.isMigModeChangePending(bar0), nil
}
