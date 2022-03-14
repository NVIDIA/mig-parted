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

package config

import (
	"fmt"
	"strings"

	"github.com/NVIDIA/mig-parted/internal/nvlib"
	"github.com/NVIDIA/mig-parted/internal/nvml"
	"github.com/NVIDIA/mig-parted/pkg/types"
	log "github.com/sirupsen/logrus"
)

type Manager interface {
	GetMigConfig(gpu int) (types.MigConfig, error)
	SetMigConfig(gpu int, config types.MigConfig) error
	ClearMigConfig(gpu int) error
}

type nvmlMigConfigManager struct {
	nvml  nvml.Interface
	nvlib nvlib.Interface
}

var _ Manager = (*nvmlMigConfigManager)(nil)

func tryNvmlShutdown(nvmlLib nvml.Interface) {
	ret := nvmlLib.Shutdown()
	if ret.Value() != nvml.SUCCESS {
		log.Warnf("Error shutting down NVML: %v", ret)
	}
}

func NewNvmlMigConfigManager() Manager {
	return &nvmlMigConfigManager{nvml.New(), nvlib.New()}
}

func (m *nvmlMigConfigManager) GetMigConfig(gpu int) (types.MigConfig, error) {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return nil, fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
	if ret.Value() != nvml.SUCCESS {
		return nil, fmt.Errorf("error getting device handle: %v", ret)
	}

	err := m.nvlib.Mig.Device(device).AssertMigEnabled()
	if err != nil {
		return nil, fmt.Errorf("error asserting MIG enabled: %v", err)
	}

	migConfig := types.MigConfig{}
	err = m.nvlib.Mig.Device(device).WalkGpuInstances(func(gi nvml.GpuInstance, giProfileId int, giProfileInfo nvml.GpuInstanceProfileInfo) error {
		err := m.nvlib.Mig.GpuInstance(gi).WalkComputeInstances(func(ci nvml.ComputeInstance, ciProfileId int, ciEngProfileId int, ciProfileInfo nvml.ComputeInstanceProfileInfo) error {
			mp := types.NewMigProfile(giProfileId, ciProfileId, ciEngProfileId, &giProfileInfo, &ciProfileInfo)
			migConfig[mp.String()]++
			return nil
		})
		if err != nil {
			return fmt.Errorf("error walking compute instances for '%v': %v", giProfileId, err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error walking gpu instances for '%v': %v", gpu, err)
	}

	return migConfig, nil
}

func (m *nvmlMigConfigManager) SetMigConfig(gpu int, config types.MigConfig) error {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
	if ret.Value() != nvml.SUCCESS {
		return fmt.Errorf("error getting device handle: %v", ret)
	}

	err := m.nvlib.Mig.Device(device).AssertMigEnabled()
	if err != nil {
		return fmt.Errorf("error asserting MIG enabled: %v", err)
	}

	err = iteratePermutationsUntilSuccess(config, func(mps []*types.MigProfile) error {
		clearAttempts := 0
		maxClearAttempts := 1
		for {
			existingConfig, err := m.GetMigConfig(gpu)
			if err != nil {
				return fmt.Errorf("error getting existing MigConfig: %v", err)
			}

			if len(existingConfig.Flatten()) == 0 {
				break
			}

			if clearAttempts == maxClearAttempts {
				return fmt.Errorf("exceeded maximum attempts to clear MigConfig")
			}

			err = m.ClearMigConfig(gpu)
			if err != nil {
				return fmt.Errorf("error clearing MigConfig: %v", err)
			}

			clearAttempts++
		}

		var lastGIProfileId int = -1
		var gi nvml.GpuInstance = nil
		for _, mp := range mps {
			giProfileInfo, ret := device.GetGpuInstanceProfileInfo(mp.GIProfileId)
			if ret.Value() != nvml.SUCCESS {
				return fmt.Errorf("error getting GPU instance profile info for '%v': %v", mp, ret)
			}

			reuseGI := (gi != nil) && (lastGIProfileId == mp.GIProfileId)
			lastGIProfileId = mp.GIProfileId

			for {
				if !reuseGI {
					gi, ret = device.CreateGpuInstance(&giProfileInfo)
					if ret.Value() != nvml.SUCCESS {
						return fmt.Errorf("error creating GPU instance for '%v': %v", mp, ret)
					}
				}

				ciProfileInfo, ret := gi.GetComputeInstanceProfileInfo(mp.CIProfileId, mp.CIEngProfileId)
				if ret.Value() != nvml.SUCCESS {
					if reuseGI {
						reuseGI = false
						continue
					}
					return fmt.Errorf("error getting Compute instance profile info for '%v': %v", mp, ret)
				}

				_, ret = gi.CreateComputeInstance(&ciProfileInfo)
				if ret.Value() != nvml.SUCCESS {
					if reuseGI {
						reuseGI = false
						continue
					}
					return fmt.Errorf("error creating Compute instance for '%v': %v", mp, ret)
				}

				valid := types.NewMigProfile(mp.GIProfileId, mp.CIProfileId, mp.CIEngProfileId, &giProfileInfo, &ciProfileInfo)
				if !mp.Equals(valid) {
					if reuseGI {
						reuseGI = false
						continue
					}
					return fmt.Errorf("unsupported MIG Device specified %v, expected %v instead", mp, valid)
				}

				break
			}
		}

		return nil
	})
	if err != nil {
		e := m.ClearMigConfig(gpu)
		if e != nil {
			log.Errorf("Error clearing MIG config on GPU %d, erroneous devices may persist", gpu)
		}
		return fmt.Errorf("error attempting multiple config orderings: %v", err)
	}

	return nil
}

func (m *nvmlMigConfigManager) ClearMigConfig(gpu int) error {
	ret := m.nvml.Init()
	if ret.Value() != nvml.SUCCESS {
		return fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer tryNvmlShutdown(m.nvml)

	device, ret := m.nvml.DeviceGetHandleByIndex(gpu)
	if ret.Value() != nvml.SUCCESS {
		return fmt.Errorf("error getting device handle: %v", ret)
	}

	err := m.nvlib.Mig.Device(device).AssertMigEnabled()
	if err != nil {
		return fmt.Errorf("error asserting MIG enabled: %v", err)
	}

	err = m.nvlib.Mig.Device(device).WalkGpuInstances(func(gi nvml.GpuInstance, giProfileId int, giProfileInfo nvml.GpuInstanceProfileInfo) error {
		err := m.nvlib.Mig.GpuInstance(gi).WalkComputeInstances(func(ci nvml.ComputeInstance, ciProfileId int, ciEngProfileId int, ciProfileInfo nvml.ComputeInstanceProfileInfo) error {
			ret := ci.Destroy()
			if ret.Value() != nvml.SUCCESS {
				return fmt.Errorf("error destroying Compute instance for profile '(%v, %v)': %v", ciProfileId, ciEngProfileId, ret)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("error walking compute instances for '%v': %v", giProfileId, err)
		}

		ret := gi.Destroy()
		if ret.Value() != nvml.SUCCESS {
			return fmt.Errorf("error destroying GPU instance for profile '%v': %v", giProfileId, ret)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking gpu instances for '%v': %v", gpu, err)
	}
	return nil
}

func iteratePermutationsUntilSuccess(config types.MigConfig, f func([]*types.MigProfile) error) error {
	shouldSwap := func(mps []*types.MigProfile, start, curr int) bool {
		for i := start; i < curr; i++ {
			if mps[i] == mps[curr] {
				return false
			}
		}
		return true
	}

	var iterate func(mps []*types.MigProfile, f func([]*types.MigProfile) error, index int) error
	iterate = func(mps []*types.MigProfile, f func([]*types.MigProfile) error, i int) error {
		if i >= len(mps) {
			err := f(mps)
			if err != nil {
				e := err.Error()
				log.Error(strings.ToUpper(e[0:1]) + e[1:])
			}
			return err
		}

		for j := i; j < len(mps); j++ {
			if shouldSwap(mps, i, j) {
				mps[i], mps[j] = mps[j], mps[i]

				err := iterate(mps, f, i+1)
				if err == nil {
					return nil
				}

				mps[i], mps[j] = mps[j], mps[i]
			}
		}

		return fmt.Errorf("all orderings failed")
	}

	return iterate(config.Flatten(), f, 0)
}
