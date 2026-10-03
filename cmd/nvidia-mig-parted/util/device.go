/*
 * Copyright (c) 2023, NVIDIA CORPORATION.  All rights reserved.
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

package util

import (
	"fmt"
	"os/exec"
	"strings"

	nvdevlib "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvlib/pkg/nvpci"
	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/NVIDIA/mig-parted/pkg/types"
)

func GetGPUDeviceIDs() ([]types.DeviceID, error) {
	nvidiaModuleLoaded, err := IsNvidiaModuleLoaded()
	if err != nil {
		return nil, fmt.Errorf("error checking if nvidia module loaded: %v", err)
	}
	if nvidiaModuleLoaded {
		return nvmlGetGPUDeviceIDs()
	}
	return pciGetGPUDeviceIDs()
}

func ResetAllGPUs() (string, error) {
	deviceIDs, err := GetGPUDeviceIDs()
	if err != nil {
		return "", fmt.Errorf("error enumerating GPUs: %v", err)
	}
	devicesToReset := make([]int, len(deviceIDs))
	for i := range deviceIDs {
		devicesToReset[i] = i
	}
	return ResetGPUs(devicesToReset)
}

func ResetGPUs(devicesToReset []int) (string, error) {
	nvidiaModuleLoaded, err := IsNvidiaModuleLoaded()
	if err != nil {
		return "", fmt.Errorf("error checking if nvidia module loaded: %v", err)
	}

	if nvidiaModuleLoaded {
		return nvmlResetGPUs(devicesToReset)
	}
	return pciResetGPUs(devicesToReset)
}

func pciVisitGPUs(visit func(*nvpci.NvidiaPCIDevice) error) error {
	nvpci := nvpci.New()
	gpus, err := nvpci.GetGPUs()
	if err != nil {
		return fmt.Errorf("error enumerating GPUs: %v", err)
	}
	for _, gpu := range gpus {
		err := visit(gpu)
		if err != nil {
			return err
		}
	}
	return nil
}

func pciGetGPUDeviceIDs() ([]types.DeviceID, error) {
	var ids []types.DeviceID
	err := pciVisitGPUs(func(gpu *nvpci.NvidiaPCIDevice) error {
		ids = append(ids, types.NewDeviceIDWithSubsystem(gpu.Device, gpu.Vendor, gpu.SubsystemDevice, gpu.SubsystemVendor))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func nvmlGetGPUDeviceIDs() ([]types.DeviceID, error) {
	nvmlLib := nvml.New()
	err := NvmlInit(nvmlLib)
	if err != nil {
		return nil, fmt.Errorf("error initializing NVML: %v", err)
	}
	defer TryNvmlShutdown(nvmlLib)

	var ids []types.DeviceID
	err = pciVisitGPUs(func(gpu *nvpci.NvidiaPCIDevice) error {
		_, ret := nvmlLib.DeviceGetHandleByPciBusId(gpu.Address)
		if ret != nvml.SUCCESS {
			return nil
		}

		ids = append(ids, types.NewDeviceIDWithSubsystem(gpu.Device, gpu.Vendor, gpu.SubsystemDevice, gpu.SubsystemVendor))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func pciResetGPUs(devicesToReset []int) (string, error) {
	nvpciLib := nvpci.New()
	for _, index := range devicesToReset {
		gpu, err := nvpciLib.GetGPUByIndex(index)
		if err != nil {
			return "", fmt.Errorf("error getting GPU %d: %v", index, err)
		}
		if err := gpu.Reset(); err != nil {
			return "", fmt.Errorf("error resetting GPU %v: %v", gpu.Address, err)
		}
	}
	return "", nil
}

func nvmlGetGPUPciBusIds(devicesToReset []int) ([]string, error) {
	nvmlLib := nvml.New()
	err := NvmlInit(nvmlLib)
	if err != nil {
		return nil, fmt.Errorf("error initializing NVML: %v", err)
	}
	defer TryNvmlShutdown(nvmlLib)

	var ids []string
	for _, index := range devicesToReset {
		gpu, err := nvmlGetGPUByIndex(nvmlLib, index)
		if err != nil {
			return nil, err
		}
		ids = append(ids, gpu.Address)
	}

	return ids, nil
}

func nvmlResetGPUs(devicesToReset []int) (string, error) {
	pciBusIDs, err := nvmlGetGPUPciBusIds(devicesToReset)
	if err != nil {
		return "", fmt.Errorf("error getting GPU pci bus IDs: %v", err)
	}

	if len(pciBusIDs) == 0 {
		return "No GPUs to reset...", nil
	}

	cmd := exec.Command("nvidia-smi", "-r", "-i", strings.Join(pciBusIDs, ",")) //nolint:gosec
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func nvmlGetGPUByIndex(nvmlLib nvml.Interface, index int) (*nvpci.NvidiaPCIDevice, error) {
	device, ret := nvmlLib.DeviceGetHandleByIndex(index)
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting GPU %d: %v", index, ret)
	}

	nvdev, err := nvdevlib.New(nvmlLib).NewDevice(device)
	if err != nil {
		return nil, fmt.Errorf("error wrapping GPU %d: %v", index, err)
	}

	address, err := nvdev.GetPCIBusID()
	if err != nil {
		return nil, fmt.Errorf("error getting PCI bus ID for GPU %d: %v", index, err)
	}

	gpu, err := nvpci.New().GetGPUByPciBusID(address)
	if err != nil {
		return nil, fmt.Errorf("error getting PCI device %s for GPU %d: %v", address, index, err)
	}
	if gpu == nil {
		return nil, fmt.Errorf("no GPU found at %s for GPU %d", address, index)
	}

	return gpu, nil
}
