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
	"strings"

	"github.com/NVIDIA/mig-parted/pkg/mig/config"
	"github.com/NVIDIA/mig-parted/pkg/mig/mode"
)

type CombinedMigManager interface {
	mode.Manager
	config.Manager
}

func NewCombinedMigManager() CombinedMigManager {
	type modeManager = mode.Manager
	type configManager = config.Manager
	return &struct {
		modeManager
		configManager
	}{mode.NewPciMigModeManager(), config.NewNvmlMigConfigManager()}
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
