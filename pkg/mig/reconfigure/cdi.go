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

package reconfigure

import (
	"fmt"
	"os/exec"

	"github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi"
	transformroot "github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi/transform/root"
	log "github.com/sirupsen/logrus"
)

func (opts *reconfigurer) regenerateManagementCDISpec() error {
	log.Info("Generating CDI spec for management containers")
	cdilib, err := nvcdi.New(
		nvcdi.WithMode(nvcdi.ModeManagement),
		nvcdi.WithDriverRoot(opts.DriverRootCtrPath),
		nvcdi.WithDevRoot(opts.DevRootCtrPath),
		nvcdi.WithNVIDIACDIHookPath(opts.NVIDIACDIHookPath),
		nvcdi.WithVendor("management.nvidia.com"),
		nvcdi.WithClass("gpu"),
	)
	if err != nil {
		return fmt.Errorf("failed to create CDI library for management containers: %v", err)
	}

	spec, err := cdilib.GetSpec()
	if err != nil {
		return fmt.Errorf("failed to genereate CDI spec for management containers: %v", err)
	}

	transformer := transformroot.NewDriverTransformer(
		transformroot.WithDriverRoot(opts.DriverRootCtrPath),
		transformroot.WithTargetDriverRoot(opts.DriverRoot),
		transformroot.WithDevRoot(opts.DevRootCtrPath),
		transformroot.WithTargetDevRoot(opts.DevRoot),
	)
	if err := transformer.Transform(spec.Raw()); err != nil {
		return fmt.Errorf("failed to transform driver root in CDI spec: %v", err)
	}
	err = spec.Save("/var/run/cdi/management.nvidia.com-gpu.yaml")
	if err != nil {
		return fmt.Errorf("failed to save CDI spec for management containers: %v", err)
	}

	return nil
}

// TODO: Instead of shelling out like this, we should either expose the API or
// just create the missing nvidia-uvm and nvidia-uvm-tools nodes.
func (opts *reconfigurer) createControlDeviceNodes() error {
	args := []string{
		"system", "create-device-nodes",
		"--control-devices",
		"--dev-root=" + opts.DevRootCtrPath,
	}
	cmd := exec.Command("nvidia-ctk", args...)

	return opts.Run(cmd)
}

func (opts *reconfigurer) runNvidiaSMI() error {
	if opts.DriverRootCtrPath == opts.DevRootCtrPath {
		cmd := exec.Command("chroot", opts.DriverRootCtrPath, opts.NVIDIASMIPath)
		return opts.Run(cmd)
	}

	cmd := exec.Command("chroot", opts.HostRootMount, opts.NVIDIASMIPath)
	cmd.Env = append(cmd.Env, ldPreloadEnvVar+"="+opts.DriverLibraryPath)
	return opts.Run(cmd)
}
