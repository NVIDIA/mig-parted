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

package types

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	nvdev "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
)

var nvmllib nvml.Interface
var nvdevlib nvdev.Interface

func tryNvmlShutdown(nvmlLib nvml.Interface) {
	ret := nvmlLib.Shutdown()
	if ret != nvml.SUCCESS {
		log.Warnf("Error shutting down NVML: %v", ret)
	}
}

func nvdevNewMigProfile(giProfileID, ciProfileID, ciEngProfileID int, migMemorySizeMB, deviceMemorySizeBytes uint64) (nvdev.MigProfile, error) {
	if nvmllib == nil {
		nvmllib = nvml.New()
	}
	if nvdevlib == nil {
		nvdevlib = nvdev.New(nvdev.WithNvml(nvmllib))
	}

	mp, err := nvdevlib.NewMigProfile(giProfileID, ciProfileID, ciEngProfileID, migMemorySizeMB, deviceMemorySizeBytes)
	if err != nil {
		return nil, err
	}

	return mp, nil
}

func nvdevAssertValidMigProfileFormat(profile string) error {
	if nvmllib == nil {
		nvmllib = nvml.New()
	}
	if nvdevlib == nil {
		nvdevlib = nvdev.New(nvdev.WithNvml(nvmllib))
	}

	return nvdevlib.AssertValidMigProfileFormat(profile)
}

func nvdevParseMigProfile(profile string) (nvdev.MigProfile, error) {
	if nvmllib == nil {
		nvmllib = nvml.New()
	}
	if nvdevlib == nil {
		nvdevlib = nvdev.New(nvdev.WithNvml(nvmllib))
	}

	ret := nvmllib.Init()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(nvmllib)

	mp, err := nvdevlib.ParseMigProfile(profile)
	if err != nil {
		return nil, err
	}

	return mp, nil
}

func SetMockNVdevlib() {
	mockDevice := &mock.Device{
		GetNameFunc: func() (string, nvml.Return) {
			return "MockDevice", nvml.SUCCESS
		},
		GetMigModeFunc: func() (int, int, nvml.Return) {
			return nvml.DEVICE_MIG_ENABLE, nvml.DEVICE_MIG_ENABLE, nvml.SUCCESS
		},
		GetMemoryInfoFunc: func() (nvml.Memory, nvml.Return) {
			memory := nvml.Memory{
				Total: 40 * 1024 * 1024 * 1024,
			}
			return memory, nvml.SUCCESS
		},
		GetGpuInstanceProfileInfoFunc: func(Profile int) (nvml.GpuInstanceProfileInfo, nvml.Return) {
			info := nvml.GpuInstanceProfileInfo{}
			switch Profile {
			case nvml.GPU_INSTANCE_PROFILE_1_SLICE,
				nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV1:
				info.MemorySizeMB = 5 * 1024
			case nvml.GPU_INSTANCE_PROFILE_1_SLICE_REV2:
				info.MemorySizeMB = 10 * 1024
			case nvml.GPU_INSTANCE_PROFILE_2_SLICE,
				nvml.GPU_INSTANCE_PROFILE_2_SLICE_REV1:
				info.MemorySizeMB = 10 * 1024
			case nvml.GPU_INSTANCE_PROFILE_3_SLICE:
				info.MemorySizeMB = 20 * 1024
			case nvml.GPU_INSTANCE_PROFILE_4_SLICE:
				info.MemorySizeMB = 20 * 1024
			case nvml.GPU_INSTANCE_PROFILE_7_SLICE:
				info.MemorySizeMB = 40 * 1024
			case nvml.GPU_INSTANCE_PROFILE_6_SLICE,
				nvml.GPU_INSTANCE_PROFILE_8_SLICE:
				fallthrough
			default:
				return info, nvml.ERROR_NOT_SUPPORTED
			}
			return info, nvml.SUCCESS
		},
	}

	nvmllib = &mock.Interface{
		InitFunc: func() nvml.Return {
			return nvml.SUCCESS
		},
		ShutdownFunc: func() nvml.Return {
			return nvml.SUCCESS
		},
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 1, nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(Index int) (nvml.Device, nvml.Return) {
			return mockDevice, nvml.SUCCESS
		},
	}

	nvdevlib = nvdev.New(
		nvdev.WithNvml(nvmllib),
		nvdev.WithVerifySymbols(false),
	)
}
