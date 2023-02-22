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
	"os"
	"strconv"
	"strings"

	"github.com/NVIDIA/mig-parted/internal/nvml"
	log "github.com/sirupsen/logrus"
)

const (
	minSupportedNVML = 11
)

func IsNvidiaModuleLoaded() (bool, error) {
	modules, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false, fmt.Errorf("unable to read /proc/modules: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(modules)), "\n") {
		fields := strings.Fields(line)
		if fields[0] == "nvidia" {
			return true, nil
		}
	}
	return false, nil
}

func IsNVMLVersionSupported() (bool, error) {
	nvmlLib := nvml.New()

	ret := nvmlLib.Init()
	if ret.Value() != nvml.SUCCESS {
		return false, fmt.Errorf("error initializing NVML: %v", ret)
	}
	defer func() {
		ret := nvmlLib.Shutdown()
		if ret.Value() != nvml.SUCCESS {
			log.Warnf("error shutting down NVML: %v", ret)
		}
	}()

	sversion, ret := nvmlLib.SystemGetNVMLVersion()
	if ret.Value() != nvml.SUCCESS {
		return false, fmt.Errorf("error getting getting version: %v", ret)
	}

	split := strings.Split(sversion, ".")
	if len(split) == 0 {
		return false, fmt.Errorf("unexpected empty version string")
	}

	iversion, err := strconv.Atoi(split[0])
	if err != nil {
		return false, fmt.Errorf("malformed version string '%s': %v", sversion, err)
	}

	if iversion < minSupportedNVML {
		return false, nil
	}

	return true, nil
}

func NvmlInit(nvmlLib nvml.Interface) error {
	if nvmlLib == nil {
		nvmlLib = nvml.New()
	}
	ret := nvmlLib.Init()
	if ret.Value() != nvml.SUCCESS {
		return ret
	}
	return nil
}

func TryNvmlShutdown(nvmlLib nvml.Interface) {
	if nvmlLib == nil {
		nvmlLib = nvml.New()
	}
	ret := nvmlLib.Shutdown()
	if ret.Value() != nvml.SUCCESS {
		log.Warnf("error shutting down NVML: %v", ret)
	}
}
