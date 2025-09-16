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
	"k8s.io/client-go/kubernetes"
)

// An Option represents a functional option passed to the constructor.
type Option func(*reconfigureMIGOptions)

// reconfigureMIGOptions contains configuration options for reconfiguring MIG
// settings on a Kubernetes node. This struct is used to manage the various
// parameters required for applying MIG configurations through mig-parted, including node identification, configuration files, reboot behavior, and host
// system service management.
type reconfigureMIGOptions struct {
	clientset *kubernetes.Clientset `validate:"required"`

	// NodeName is the kubernetes node to change the MIG configuration on.
	// Its validation follows the RFC 1123 standard for DNS subdomain names.
	// Source: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
	NodeName string `validate:"required,hostname_rfc1123"`

	// MIGPartedConfigFile is the mig-parted configuration file path.
	MIGPartedConfigFile string `validate:"required,filepath"`

	// SelectedMIGConfig is the selected mig-parted configuration to apply to the
	// node.
	// TODO: Define the validation schema.
	SelectedMIGConfig string `validate:"required"`

	// DriverLibraryPath is the path to libnvidia-ml.so.1 in the container.
	DriverLibraryPath string `validate:"required,filepath"`

	// WithReboot reboots the node if changing the MIG mode fails for any reason.
	WithReboot bool

	// WithShutdownHostGPUClients shutdowns/restarts any required host GPU clients
	// across a MIG configuration.
	WithShutdownHostGPUClients bool

	// HostRootMount is the container path where host root directory is mounted.
	HostRootMount string `validate:"dirpath"`

	// HostMIGManagerStateFile is the path where the systemd mig-manager state
	// file is located.
	HostMIGManagerStateFile string `validate:"omitempty,filepath"`

	// HostGPUClientServices is a comma separated list of host systemd services to
	// shutdown/restart across a MIG reconfiguration.
	HostGPUClientServices []string `validate:"dive,systemd_service_name"`

	// HostKubeletService is the name of the host's 'kubelet' systemd service
	// which may need to be shutdown/restarted across a MIG mode reconfiguration.
	HostKubeletService string `validate:"omitempty,systemd_service_name"`

	// TODO: Define the validation schema.
	ConfigStateLabel string `validate:"required"`

	// TODO: The following is not an option, but is tracked during the reconfiguration.
	hostGPUClientServicesStopped []string
}

func WithAllowReboot(allowReboot bool) Option {
	return func(o *reconfigureMIGOptions) {
		o.WithReboot = allowReboot
	}
}

func WithClientset(clientset *kubernetes.Clientset) Option {
	return func(o *reconfigureMIGOptions) {
		o.clientset = clientset
	}
}

func WithConfigStateLabel(configStateLabel string) Option {
	return func(o *reconfigureMIGOptions) {
		o.ConfigStateLabel = configStateLabel
	}
}

func WithDriverLibraryPath(driverLibraryPath string) Option {
	return func(o *reconfigureMIGOptions) {
		o.DriverLibraryPath = driverLibraryPath
	}
}

func WithHostGPUClientServices(hostGPUClientServices ...string) Option {
	return func(o *reconfigureMIGOptions) {
		o.HostGPUClientServices = append([]string{}, hostGPUClientServices...)
	}
}

func WithHostKubeletService(hostKubeletService string) Option {
	return func(o *reconfigureMIGOptions) {
		o.HostKubeletService = hostKubeletService
	}
}

func WithHostMIGManagerStateFile(hostMIGManagerStateFile string) Option {
	return func(o *reconfigureMIGOptions) {
		o.HostMIGManagerStateFile = hostMIGManagerStateFile
	}
}

func WithHostRootMount(hostRootMount string) Option {
	return func(o *reconfigureMIGOptions) {
		o.HostRootMount = hostRootMount
	}
}

func WithMIGPartedConfigFile(migPartedConfigFile string) Option {
	return func(o *reconfigureMIGOptions) {
		o.MIGPartedConfigFile = migPartedConfigFile
	}
}

func WithNodeName(nodeName string) Option {
	return func(o *reconfigureMIGOptions) {
		o.NodeName = nodeName
	}
}

func WithSelectedMIGConfig(selectedMIGConfig string) Option {
	return func(o *reconfigureMIGOptions) {
		o.SelectedMIGConfig = selectedMIGConfig
	}
}

func WithShutdownHostGPUClients(shutdownHostGPUClients bool) Option {
	return func(o *reconfigureMIGOptions) {
		o.WithShutdownHostGPUClients = shutdownHostGPUClients
	}
}
