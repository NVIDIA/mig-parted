/**
# SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package main

import "os/exec"

func createControlDeviceNodes(opts *reconfigureMIGOptions) error {
	args := []string{
		"system", "create-device-nodes",
		"--control-devices",
		"--dev-root=" + opts.DevRootCtrPath,
	}
	cmd := exec.Command("nvidia-ctk", args...)

	return runCommandWithOutput(cmd)
}

func runNvidiaSMI(opts *reconfigureMIGOptions) error {
	if opts.DriverRootCtrPath == opts.DevRootCtrPath {
		cmd := exec.Command("chroot", opts.NVIDIASMIPath)
		return runCommandWithOutput(cmd)
	}

	cmd := exec.Command("chroot", opts.HostRootMount, opts.NVIDIASMIPath)
	cmd.Env = append(cmd.Env, ldPreloadEnvVar+"="+opts.DriverLibraryPath)
	return runCommandWithOutput(cmd)
}
