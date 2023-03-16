// #!/usr/bin/env bash

// # Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
// #
// # Licensed under the Apache License, Version 2.0 (the "License");
// # you may not use this file except in compliance with the License.
// # You may obtain a copy of the License at
// #
// #     http://www.apache.org/licenses/LICENSE-2.0
// #
// # Unless required by applicable law or agreed to in writing, software
// # distributed under the License is distributed on an "AS IS" BASIS,
// # WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// # See the License for the specific language governing permissions and
// # limitations under the License.

package reconfigureMig

import (
	"fmt"
	"os"
	"os/exec"
)

var (
	WITH_REBOOT                    = false
	WITH_SHUTDOWN_HOST_GPU_CLIENTS = false
	HOST_ROOT_MOUNT                = ""
	HOST_NVIDIA_DIR                = ""
	HOST_MIG_MANAGER_STATE_FILE    = ""
	HOST_GPU_CLIENT_SERVICES       = ""
	HOST_KUBELET_SERVICE           = ""
	NODE_NAME                      = ""
	MIG_CONFIG_FILE                = ""
	SELECTED_MIG_CONFIG            = ""
	DEFAULT_GPU_CLIENTS_NAMESPACE  = ""
	systemdloglevel                = "info"
)

func usage() {
	fmt.Println("USAGE:")
	fmt.Printf(" %s -h\n", os.Args[0])
	fmt.Printf(" %s -n <node> -f <config-file> -c <selected-config> -p <default-gpu-clients-namespace> [ -m <host-root-mount> -i <host-nvidia-dir> -o <host-mig-manager-state-file> -g <host-gpu-client-services> -k <host-kubelet-service> -r -s ] \n", os.Args[0])
	fmt.Println("")
	fmt.Println("OPTIONS:")
	fmt.Println("    -h                                   Display this help message")
	fmt.Println("    -r                                   Automatically reboot the node if changing the MIG mode fails for any reason")
	fmt.Println("    -d                                   Automatically shutdown/restart any required host GPU clients across a MIG configuration")
	fmt.Println("    -n <node>                            The kubernetes node to change the MIG configuration on")
	fmt.Println("    -f <config-file>                     The mig-parted configuration file")
	fmt.Println("    -c <selected-config>                 The selected mig-parted configuration to apply to the node")
	fmt.Println("    -m <host-root-mount>                 Container path where host root directory is mounted")
	fmt.Println("    -i <host-nvidia-dir>                 Host path of the directory where NVIDIA managed software directory is typically located")
	fmt.Println("    -o <host-mig-manager-state-file>     Host path where the systemd mig-manager state file is located")
	fmt.Println("    -g <host-gpu-client-services>        Comma separated list of host systemd services to shutdown/restart across a MIG reconfiguration")
	fmt.Println("    -k <host-kubelet-service>            Name of the host's 'kubelet' systemd service which may need to be shutdown/restarted across a MIG mode reconfiguration")
	fmt.Println("    -p <default-gpu-clients-namespace>   Default name of the Kubernetes Namespace in which the GPU client Pods are installed in")
}

if NODE_NAME == "" {
	fmt.Println("ERROR: missing -n <node> flag")
	usage()
	os.Exit(1)
}

if MIG_CONFIG_FILE = "" {
	fmt.Println("Error: missing -f <config-file> flag")
	usage()
	os.Exit(1)
}

if SELECTED_MIG_CONFIG = "" {
	fmt.Println("Error: missing -c <selected-config> flag")
	usage()
	os.Exit(1)
}

if DEFAULT_GPU_CLIENTS_NAMESPACE "" {
	fmt.Println("Error: missing -p <default-gpu-clients-namespace> flag")
	usage()
	os.Exit(1)
}

func __set_state_and_exit() {
	var state string = ""
	var exit_code uint = ""
	if WITH_SHUTDOWN_HOST_GPU_CLIENTS == "true" {
		if NO_RESTART_HOST_SYSTEMD_SERVICES_ON_EXIT != "true" {
			fmt.Println("Restarting any GPU clients previously shutdown on the host by restarting their systemd services")
			host_start_systemd_services()
			if exit_code != 0 {
				fmt.Println("Unable to restart host systemd services")
				exit_code = 1
			}
		}
	}

	if NO_RESTART_K8S_DAEMONSETS_ON_EXIT != "true" {
		fmt.Println("Restarting any GPU clients previously shutdown in Kubernetes by reenabling their component-specific nodeSelector labels")
		kubectlLabel := []string{"node", NODE_NAME, "nvidia.com/gpu.deploy.device-plugin" + maybe_set_true(PLUGIN_DEPLOYED), "nvidia.com/gpu.deploy.gpu-feature-discovery" + maybe_set_true(GFD_DEPLOYED), "nvidia.com/gpu.deploy.dcgm-exporter" + maybe_set_true(DCGM_EXPORTER_DEPLOYED), "nvidia.com/gpu.deploy.dcgm" + maybe_set_true(DCGM_DEPLOYED)}
		cmd := exec.Command("kubectl", kubectlLabel)
		if err := cmd.Run(); err != nil {
			fmt.Println("Unable to bring up GPU client pods by setting their daemonset labels")
			exit_code = 1
		}
	}

	fmt.Printf("Changing the 'nvidia.com/mig.config.state' node label to '%s' \n", state)
	kubectlLabel := []string{"node", NODE_NAME, "nvidia.com/mig.config.state=" + state}
	cmd := exec.Command("kubectl", append([]string{"label", "--overwrite"}, kubectlLabel))
	if err := cmd.Run(); err != nil {
		fmt.Printf("Unable to set 'nvidia.com/mig.config.state' to '%s'\n", state)
		fmt.Println("Exiting with incorrect value in 'nvidia.com/mig.config.state'")
		exit_code = 1
	}
	os.Exit(exit_code)
}

func exit_success() {

}

func exit_failed() {

}

func maybe_set_paused() {

}

func maybe_set_true() {

}

func host_stop_systemd_services() {

}

func host_start_systemd_services() {

}

func host_persist_config() {

}