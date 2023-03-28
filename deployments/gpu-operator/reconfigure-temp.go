/**
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
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

import (
	"fmt"
	"os"
	"os/exec"
)

var (
	WITH_REBOOT                   = "false"
	withShutdownHostGpuClinets    = "false"
	hostRootMount                 = ""
	hostNvidiaDir                 = ""
	HOST_MIG_MANAGER_STATE_FILE   = ""
	HOST_GPU_CLIENT_SERVICES      = ""
	HOST_KUBELET_SERVICE          = ""
	NODE_NAME                     = ""
	migConfigFile                 = ""
	SELECTED_MIG_CONFIG           = ""
	DEFAULT_GPU_CLIENTS_NAMESPACE = ""
)

// reconfgiure
func reconfigure(selectedConfig string) error {
	configFile, err := initMigParted()
	if err != nil {
		return err
	}

	currentLabels, err := getCurrentLabels()
	if err != nil {
		return err
	}

	if err := assertConfigPresent(configFile, selectedConfig); err != nil {
		return err
	}

	if err := setStateLabel(); err != nil {
		return err
	}

	configApplied, err := applyConfig(selectedConfig)

}

func __set_state_and_exit(arg1 string, arg2 int) {
	state := arg1
	exit_code := arg2

	if withShutdownHostGpuClinets == "true" {
		if NO_RESTART_HOST_SYSTEMD_SERVICES_ON_EXIT != "true" {
			fmt.Println("Restarting any GPU clients previously shutdown on the host by restarting their systemd services")
			ret := host_start_systemd_services()
			if ret {
				fmt.Println("Unable to restart host systemd services")
				exit_code = 1
			}
		}
	}

	if NO_RESTART_K8S_DAEMONSETS_ON_EXIT != "true" {
		fmt.Println("Restarting any GPU clients previously shutdown in Kubernetes by reenabling their component-specific nodeSelector labels")
		cmd := fmt.Sprintf("kubectl label --overwrite node %s nvidia.com/gpu.deploy.device-plugin=$(maybe_set_true %s) nvidia.com/gpu.deploy.gpu-feature-discovery=$(maybe_set_true %s) nvidia.com/gpu.deploy.dcgm-exporter=$(maybe_set_true %s) nvidia.com/gpu.deploy.dcgm=$(maybe_set_true %s)", NODE_NAME, PLUGIN_DEPLOYED, GFD_DEPLOYED, DCGM_EXPORTER_DEPLOYED, DCGM_DEPLOYED)
		_, err = getCmdOutput(cmd)
		if err != "0" {
			fmt.Println("Unable to bring up GPU client pods by setting their daemonset labels")
			exit_code = 1
		}
	}

	fmt.Println("Changing the 'nvidia.com/mig.config.state' node label to '%s'", state)
	cmd := fmt.Sprintf("label --overwrite node %s nvidia.com/mig.config.state='%s'", NODE_NAME, state)
	_, err = getCmdOutput(cmd)
	if err != "0" {
		fmt.Println("Unable to set 'nvidia.com/mig.config.state' to '%s'", state)
		fmt.Println("Exiting with incorrect value in 'nvidia.com/mig.config.state'")
		exit_code = 1

	}
}

func exit_success() {
	__set_state_and_exit("success", 0)
}

func exit_failed() {
	__set_state_and_exit("failed", 1)
}

// initMigParted sets up mig-parted on the host if required and sets the config file.
// TODO: Should implement:
/**
if [ "${WITH_SHUTDOWN_HOST_GPU_CLIENTS}" = "true" ]; then
	mkdir -p "${HOST_ROOT_MOUNT}/${HOST_NVIDIA_DIR}/mig-manager/"
	cp "$(which nvidia-mig-parted)" "${HOST_ROOT_MOUNT}/${HOST_NVIDIA_DIR}/mig-manager/"
	cp "${MIG_CONFIG_FILE}" "${HOST_ROOT_MOUNT}/${HOST_NVIDIA_DIR}/mig-manager/config.yaml"
	shopt -s expand_aliases
	alias nvidia-mig-parted="chroot ${HOST_ROOT_MOUNT} ${HOST_NVIDIA_DIR}/mig-manager/nvidia-mig-parted"
	MIG_CONFIG_FILE="${HOST_NVIDIA_DIR}/mig-manager/config.yaml"
fi
**/
func initMigParted() string {
	if withShutdownHostGpuClinets == "true" {
		os.MkdirAll(fmt.Sprintf("%s/%s/mig-manager/", hostRootMount, hostNvidiaDir), os.ModePerm)
		exec.Command("cp ${which nvidia-mig-parted}", fmt.Sprintf("%s/%s/mig-manager/nvidia-mig-parted", hostRootMount, hostNvidiaDir)).Run()
		exec.Command("cp", migConfigFile, fmt.Sprintf("%s/%s/mig-manager/config.yaml", hostRootMount, hostNvidiaDir)).Run()
		nvidiaMigPartedAlias := fmt.Sprintf("chroot %s %s/mig-manager/nvidia-mig-parted", hostRootMount, hostNvidiaDir)
		exec.Command("sh", "-c", fmt.Sprintf("alias nvidia-mig-parted='%s'", nvidiaMigPartedAlias)).Run()
		migConfigFile = fmt.Sprintf("%s/mig-manager/config.yaml", hostNvidiaDir)
		return migConfigFile
	}
	return ""
}

// getCurrentLabels
// TODO: Should implement:
/**
echo "Getting current value of the 'nvidia.com/gpu.deploy.device-plugin' node label"
PLUGIN_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.device-plugin}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.device-plugin' label"
	exit_failed
fi
echo "Current value of 'nvidia.com/gpu.deploy.device-plugin=${PLUGIN_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.gpu-feature-discovery' node label"
GFD_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.gpu-feature-discovery}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.gpu-feature-discovery' label"
	exit_failed
fi
echo "Current value of 'nvidia.com/gpu.deploy.gpu-feature-discovery=${GFD_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.dcgm-exporter' node label"
DCGM_EXPORTER_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.dcgm-exporter}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.dcgm-exporter' label"
	exit_failed
fi
echo "Current value of 'nvidia.com/gpu.deploy.dcgm-exporter=${DCGM_EXPORTER_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.dcgm' node label"
DCGM_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.dcgm}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.dcgm' label"
	exit_failed
fi
echo "Current value of 'nvidia.com/gpu.deploy.dcgm=${DCGM_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.nvsm' node label"
NVSM_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.nvsm}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.nvsm' label"
	exit_failed
fi
echo "Current value of 'nvidia.com/gpu.deploy.nvsm=${NVSM_DEPLOYED}'"
**/

func getCmdOutput(key string) (string, error) {
	cmd := exec.Command(key)
	stdout, err := cmd.Output()
	str := string(stdout)
	return str, err
}

func getCurrentLabels() (map[string]string, error) {
	components := []string{
		"device-plugin",
		"gpu-feature-discovery",
		"dcgm-exporter",
		"dcgm",
		"nvsm",
	}

	currentLabels := make(map[string]string)
	for _, component := range components {
		key := fmt.Sprintf("$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia.com/gpu.deploy.%s}')", component)
		label, err := getCmdOutput(key)
		if err != nil {
			return nil, fmt.Errorf("faile to get label %q: %v", key, err)
		}
		currentLabels[component] = label
	}

	return currentLabels, nil
}

// assertConfigPresent
// TOOD: Should implement
/**
echo "Asserting that the requested configuration is present in the configuration file"
nvidia-mig-parted assert --valid-config -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
if [ "${?}" != "0" ]; then
	echo "Unable to validate the selected MIG configuration"
	exit_failed
fi
**/
func assertConfigPresent(configFile string, selectedConfig string) error {
	_, err := getCmdOutput(fmt.Sprintf("nvidia-mig-parted assert --valid-config -f %s -c %s", configFile, selectedConfig))
	if err != nil {
		fmt.Errorf("Unable to validate the selected MIG configuration")
		exit_failed()
		return err
	}
	return nil
}
