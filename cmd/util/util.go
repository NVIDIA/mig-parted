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

package util

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"strconv"
	"strings"

	"github.com/NVIDIA/mig-parted/internal/nvml"
	"github.com/NVIDIA/mig-parted/pkg/mig/config"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
	log "github.com/sirupsen/logrus"
)

const (
	minSupportedNVML = 11
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

func Any(set []bool) bool {
	for _, s := range set {
		if s {
			return true
		}
	}
	return false
}

func CountTrue(set []bool) int {
	count := 0
	for _, s := range set {
		if s {
			count++
		}
	}
	return count
}

func Capitalize(s string) string {
	return strings.ToUpper(s[0:1]) + s[1:]
}

func IsNvidiaModuleLoaded() (bool, error) {
	modules, err := ioutil.ReadFile("/proc/modules")
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

func NvidiaSmiReset(gpus ...string) (string, error) {
	var cmd *exec.Cmd
	if len(gpus) == 0 {
		return "", fmt.Errorf("no gpus specified")
	} else {
		cmd = exec.Command("nvidia-smi", "-r", "-i", strings.Join(gpus, ","))
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}
