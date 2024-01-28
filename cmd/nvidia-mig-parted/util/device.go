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

	"github.com/NVIDIA/mig-parted/internal/nvml"
	"github.com/NVIDIA/mig-parted/pkg/types"

	"github.com/NVIDIA/go-nvlib/pkg/nvpci"
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
	nvidiaModuleLoaded, err := IsNvidiaModuleLoaded()
	if err != nil {
		return "", fmt.Errorf("error checking if nvidia module loaded: %v", err)
	}
	if nvidiaModuleLoaded {
		return nvmlResetAllGPUs()
	}
	return pciResetAllGPUs()
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
		ids = append(ids, types.NewDeviceID(gpu.Device, gpu.Vendor))
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
		if ret.Value() != nvml.SUCCESS {
			return nil
		}

		ids = append(ids, types.NewDeviceID(gpu.Device, gpu.Vendor))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func pciResetAllGPUs() (string, error) {
	err := pciVisitGPUs(func(gpu *nvpci.NvidiaPCIDevice) error {
		err := gpu.Reset()
		if err != nil {
			return fmt.Errorf("error resetting GPU %v: %v", gpu.Address, err)
		}
		return nil
	})
	return "", err
}

func nvmlGetGPUPciBusIds() ([]string, error) {
	nvmlLib := nvml.New()
	err := NvmlInit(nvmlLib)
	if err != nil {
		return nil, fmt.Errorf("error initializing NVML: %v", err)
	}
	defer TryNvmlShutdown(nvmlLib)

	var ids []string
	err = pciVisitGPUs(func(gpu *nvpci.NvidiaPCIDevice) error {
		if !gpu.Is3DController() {
			return nil
		}

		_, ret := nvmlLib.DeviceGetHandleByPciBusId(gpu.Address)
		if ret.Value() != nvml.SUCCESS {
			return nil
		}

		ids = append(ids, gpu.Address)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return ids, nil
}

func nvmlResetAllGPUs() (string, error) {
	pciBusIDs, err := nvmlGetGPUPciBusIds()
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
