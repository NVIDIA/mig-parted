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

package nvml

import (
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

type Interface interface {
	Init() Return
	Shutdown() Return
	SystemGetNVMLVersion() (string, Return)
	DeviceGetCount() (int, Return)
	DeviceGetHandleByIndex(Index int) (Device, Return)
	DeviceGetHandleByUUID(UUID string) (Device, Return)
}

type Device interface {
	GetIndex() (int, Return)
	GetUUID() (string, Return)
	GetMemoryInfo() (Memory, Return)
	GetPciInfo() (PciInfo, Return)
	SetMigMode(Mode int) (Return, Return)
	GetMigMode() (int, int, Return)
	GetGpuInstanceProfileInfo(Profile int) (GpuInstanceProfileInfo, Return)
	CreateGpuInstance(Info *GpuInstanceProfileInfo) (GpuInstance, Return)
	CreateGpuInstanceWithPlacement(Info *GpuInstanceProfileInfo, Placement *GpuInstancePlacement) (GpuInstance, Return)
	GetGpuInstances(Info *GpuInstanceProfileInfo) ([]GpuInstance, Return)
}

type GpuInstance interface {
	GetInfo() (GpuInstanceInfo, Return)
	GetComputeInstanceProfileInfo(Profile int, EngProfile int) (ComputeInstanceProfileInfo, Return)
	CreateComputeInstance(Info *ComputeInstanceProfileInfo) (ComputeInstance, Return)
	GetComputeInstances(Info *ComputeInstanceProfileInfo) ([]ComputeInstance, Return)
	Destroy() Return
}

type ComputeInstance interface {
	GetInfo() (ComputeInstanceInfo, Return)
	Destroy() Return
}

type GpuInstanceInfo struct {
	Device    Device
	Id        uint32
	ProfileId uint32
	Placement GpuInstancePlacement
}

type ComputeInstanceInfo struct {
	Device      Device
	GpuInstance GpuInstance
	Id          uint32
	ProfileId   uint32
	Placement   ComputeInstancePlacement
}

type Memory nvml.Memory
type PciInfo nvml.PciInfo
type GpuInstanceProfileInfo nvml.GpuInstanceProfileInfo
type GpuInstancePlacement nvml.GpuInstancePlacement
type ComputeInstanceProfileInfo nvml.ComputeInstanceProfileInfo
type ComputeInstancePlacement nvml.ComputeInstancePlacement
