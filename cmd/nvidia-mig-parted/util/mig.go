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

	"github.com/NVIDIA/mig-parted/pkg/mig/config"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
)

func NewMigModeManager() (mode.Manager, error) {
	nvidiaModuleLoaded, err := IsNvidiaModuleLoaded()
	if err != nil {
		return nil, fmt.Errorf("error checking if nvidia module loaded: %v", err)
	}
	if !nvidiaModuleLoaded {
		return mode.NewPciMigModeManager(), nil
	}

	nvmlSupported, err := IsNVMLVersionSupported()
	if err != nil {
		return nil, fmt.Errorf("error checking NVML version: %v", err)
	}
	if !nvmlSupported {
		return mode.NewPciMigModeManager(), nil
	}

	return mode.NewNvmlMigModeManager(), nil
}

func NewMigConfigManager() (config.Manager, error) {
	nvidiaModuleLoaded, err := IsNvidiaModuleLoaded()
	if err != nil {
		return nil, fmt.Errorf("error checking if nvidia module loaded: %v", err)
	}
	if !nvidiaModuleLoaded {
		return nil, fmt.Errorf("nvidia module not loaded")
	}

	nvmlSupported, err := IsNVMLVersionSupported()
	if err != nil {
		return nil, fmt.Errorf("error checking NVML version: %v", err)
	}
	if !nvmlSupported {
		return nil, fmt.Errorf("NVML version unsupported for performing MIG operations")
	}

	return config.NewNvmlMigConfigManager(), nil
}
