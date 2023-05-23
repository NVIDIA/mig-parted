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
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	hostGPUClientServicesStopped = []string{}
	migModeChangeRequired bool = true
	noRestartHostSystemdServicesOnExit bool
	noRestartK8sDaemonsetsOnExit bool
)

// reconfgiure == main()
func reconfigure(nodeName string, migConfigFile string, selectedMIGConfig string, hostRootMount string, hostNvidiaDir string, hostMIGManagerStateFile string, hostGPUClientService string, hostKubeletServices string, 
	defaultGPUClientsNamespace string, CDIEnabled bool, driverRoot string, driverRootCTRPath string, withReboot bool, withShutdownHostGPUClients bool) error {

	hostGPUClientServices := strings.Split(hostGPUClientService, ",")
	/*
		preparing the environment to use MIG inorder to stop GPU client services
		i/p: withShutdownHostGPUClients, hostRootMount, hostNvidiaDir, migConfigFile
		o/p: sets -> nvidia-mig-parted, migConfigFile
		exp: mig-parted should be on the container (we will be using it in our script)
	*/
	nvidiaMigPartedAlias, migConfigFile, err := initMigParted(hostRootMount, hostNvidiaDir, withShutdownHostGPUClients, migConfigFile)
	if err != nil {
		return err
	}

	/*
		retrieves current values of kubernetes node labels
		i/p: nodeName ,(device-plugin, gpu-feature-discovery, dcgm-exporter, dcgm, nvsm)
		o/p: current value of labels
	*/
	currentLabels, err := getCurrentLabels(nodeName)
	if err != nil {
		return err
	}

	/*
		requested configuration is present in the configuration file
		i/p: migConfigFile, selectedMigConfig
		o/p: true or false
	*/
	if err := assertConfigPresent(nvidiaMigPartedAlias, migConfigFile, selectedMIGConfig); err != nil {
		setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return err
	}

	/*
		capturing current value of the mig.config.state label
		i/p: nodeName
		o/p: state
	*/
	state, err := currentStateLabel(nodeName)
	if err != nil {
		setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return err
	}

	/*
		checking if selected MIG config is currently applied or not
		i/p: migConfigFile, selectedMIGConfig
		o/p: True or false
	*/
	if err := migConfigCurrentlyApplied(nvidiaMigPartedAlias, migConfigFile, selectedMIGConfig); err == nil {
		setStateAndExit("success", 0, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return nil
	}

	/*
		host_persist_config
		ensuring that right config is applied to the node if the MIG Parted is also installed as a systemd service
		i/p: hostRootMount, hostMIGManagerStateFile, selectedMIGConfig
		o/p: true or false
	*/
	// if err := rightConfigApplied(hostRootMount, hostMIGManagerStateFile, selectedMIGConfig); err != nil{
	// 	return err
	// }

	/*
		Checking if the MIG mode setting in the selected config is currently applied or not
		i/p: migConfigFile, selectedMIGConfig, STATE, withShutdownHostGPUClients, hostGPUClientServices,
		     hostKubeletServices, migModeChangeRequired
		o/p: migModeChangeRequired
	*/
	if err := migModeSettingApplied(nvidiaMigPartedAlias, migConfigFile, selectedMIGConfig); err != nil {
		if state == "rebooting" {
			fmt.Println("MIG mode change did not take effect after rebooting")
			setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
			return err
		}
		if withShutdownHostGPUClients == true {
			hostGPUClientServices = append(hostGPUClientServices, hostKubeletServices)
		}
		migModeChangeRequired = true
	}

	/*
		changing mig.config.state to pending
		i/p: nodeName
	*/
	if err := setStateToPending(nodeName); err != nil {
		fmt.Println("Unable to set the value of 'nvidia.com/mig.config.state' to 'pending'")
		setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return err
	}

	/*
		shutting down all GPU clients and deleting pods
		i/p: nodeName, currentLabels(map), withShutdownHostGPUClients, migModeChangeRequired
			 defaultGPUClientsNamespace
	*/
	if err := shuttingGPUClients(currentLabels, nodeName, defaultGPUClientsNamespace, withShutdownHostGPUClients, hostRootMount, hostGPUClientServices); err != nil {
		fmt.Println("Unable to tear down GPU client pods by setting their daemonset labels")
		return err
	}

	/*
		applies the MIG mode change from selected config file and ensuring that it is
		applied successfully
		i/p: migConfigFile, selectedMIGConfig, withReboot, nodeName, hostRootMount
	*/
	if err := applyMIGModeChange(nvidiaMigPartedAlias, migConfigFile, selectedMIGConfig, nodeName, withReboot, hostRootMount, currentLabels, hostGPUClientServices, withShutdownHostGPUClients); err != nil {
		return err
	}

	/*
		applying the selected configuration on the node
		i/p: migConfigFile, selectedMIGConfig
		o/p: true or false (error)
	*/
	if err := applyMigConfig(nvidiaMigPartedAlias, migConfigFile, selectedMIGConfig, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName); err != nil {
		return err
	}

	if err := cdiEnabled(CDIEnabled, driverRootCTRPath, driverRoot); err != nil {
		setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return err
	}

	/*
		on exit or at the end
		restarting all GPU clients (systemd services, component-specific nodeSelector)
		i/p: withShutdownHostGPUClients, noRestartHostSystemdServicesOnExit, nodeName,
			defaultGPUClientsNamespace, noRestartK8sDaemonsetsOnExit, currentLabels
	*/
	if err := restartingGPUClients(currentLabels, withShutdownHostGPUClients, defaultGPUClientsNamespace, hostRootMount, hostGPUClientServices, nodeName); err != nil {
		setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return err
	}

	setStateAndExit("success", 0, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
	return nil
}

func setStateAndExit(state string, exitCode int, currentLabels map[string]string, hostGPUClientServices []string, withShutdownHostGPUClients bool, hostRootMount string, nodeName string) {

	if withShutdownHostGPUClients == true {
		if noRestartHostSystemdServicesOnExit != true {
			fmt.Println("Restarting any GPU clients previously shutdown on the host by restarting their systemd services")
			ret := hostStartSystemdServices(hostRootMount, hostGPUClientServices)
			if ret != nil{
				fmt.Println("Unable to restart host systemd services")
				exitCode = 1
			}
		}
	}

	if noRestartK8sDaemonsetsOnExit != true {
		fmt.Println("Restarting any GPU clients previously shutdown in Kubernetes by reenabling their component-specific nodeSelector labels")
		args := []string{
			"label", "--overwrite",
			"node", nodeName,
			"nvidia.com/gpu.deploy.device-plugin=" + MaybeSetTrue(currentLabels["device-plugin"]),
			"nvidia.com/gpu.deploy.gpu-feature-discovery=" + MaybeSetTrue(currentLabels["gpu-feature-discovery"]),
			"nvidia.com/gpu.deploy.dcgm-exporter=" + MaybeSetTrue(currentLabels["dcgm-exporter"]),
			"nvidia.com/gpu.deploy.dcgm=" + MaybeSetTrue(currentLabels["dcgm"]),
		}
		cmd := exec.Command("kubectl", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Println("Unable to bring up GPU client pods by setting their daemonset labels")
			exitCode = 1
		}
	}

	fmt.Printf("Changing the 'nvidia.com/mig.config.state' node label to '%s'\n", state)
	cmd := fmt.Sprintf("label --overwrite node %s nvidia.com/mig.config.state='%s'", nodeName, state)
	_, err := getCmdOutput(cmd)
	if err != nil {
		fmt.Printf("Unable to set 'nvidia.com/mig.config.state' to '%s'\n", state)
		fmt.Println("Exiting with incorrect value in 'nvidia.com/mig.config.state'")
		exitCode = 1
	}
	os.Exit(exitCode)
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

func copyFile(srcFileName string, dstFileName string) error {
	srcFile, err := os.Open(srcFileName)
	if err != nil {
		return fmt.Errorf("error opening source file : %v", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstFileName)
	if err != nil {
		return fmt.Errorf("error creating destination file: %v", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("error writing destination file: %v", err)
	}

	return nil
}

func initMigParted(hostRootMount string, hostNvidiaDir string, withShutdownHostGpuClinets bool, migConfigFile string) (string, string, error) {
	if !withShutdownHostGpuClinets {
		return "nvidia-mig-parted", migConfigFile, nil
	}
	os.MkdirAll(fmt.Sprintf("%s/%s/mig-manager/", hostRootMount, hostNvidiaDir), 0755) 
	exec.Command("cp ${which nvidia-mig-parted}", fmt.Sprintf("%s/%s/mig-manager/nvidia-mig-parted", hostRootMount, hostNvidiaDir)).Run()
	exec.Command("cp", migConfigFile, fmt.Sprintf("%s/%s/mig-manager/config.yaml", hostRootMount, hostNvidiaDir)).Run()
	nvidiaMigPartedAlias := fmt.Sprintf("chroot %s %s/mig-manager/nvidia-mig-parted", hostRootMount, hostNvidiaDir)
	migConfigFile = fmt.Sprintf("%s/mig-manager/config.yaml", hostNvidiaDir)
	return nvidiaMigPartedAlias, migConfigFile, nil
}

// getCurrentLabels
// TODO: Should implement:
/**
echo "Getting current value of the 'nvidia.com/gpu.deploy.device-plugin' node label"
PLUGIN_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.device-plugin}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.device-plugin' label"
	exitFailed
fi
echo "Current value of 'nvidia.com/gpu.deploy.device-plugin=${PLUGIN_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.gpu-feature-discovery' node label"
GFD_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.gpu-feature-discovery}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.gpu-feature-discovery' label"
	exitFailed
fi
echo "Current value of 'nvidia.com/gpu.deploy.gpu-feature-discovery=${GFD_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.dcgm-exporter' node label"
DCGM_EXPORTER_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.dcgm-exporter}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.dcgm-exporter' label"
	exitFailed
fi
echo "Current value of 'nvidia.com/gpu.deploy.dcgm-exporter=${DCGM_EXPORTER_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.dcgm' node label"
DCGM_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.dcgm}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.dcgm' label"
	exitFailed
fi
echo "Current value of 'nvidia.com/gpu.deploy.dcgm=${DCGM_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.nvsm' node label"
NVSM_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.nvsm}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.nvsm' label"
	exitFailed
fi
echo "Current value of 'nvidia.com/gpu.deploy.nvsm=${NVSM_DEPLOYED}'"
**/

func getCmdOutput(key string) (string, error) {
	cmd := exec.Command(key)
	stdout, err := cmd.Output()
	str := string(stdout)
	return str, err
}

func getCurrentLabels(nodeName string) (map[string]string, error) {
	components := []string{
		"device-plugin",
		"gpu-feature-discovery",
		"dcgm-exporter",
		"dcgm",
		"nvsm",
	}

	currentLabels := make(map[string]string)
	for _, component := range components {
		key := fmt.Sprintf("$(kubectl get nodes ${nodeName} -o=jsonpath='{$.metadata.labels.nvidia.com/gpu.deploy.%s}')", component)
		label, err := getCmdOutput(key)
		if err != nil {
			return nil, fmt.Errorf("failed to get label %q: %v", key, err)
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
	exitFailed
fi
**/
func assertConfigPresent(nvidiaMigPartedAlias string, migConfigFile string, selectedMIGConfig string) error {
	_, err := getCmdOutput(fmt.Sprintf("%s assert --valid-config -f %s -c %s", nvidiaMigPartedAlias, migConfigFile, selectedMIGConfig))
	if err != nil {
		return fmt.Errorf("Unable to validate the selected MIG configuration: %w", err)
	}
	return nil
}

/*
currentStateLabel
echo "Getting current value of the 'nvidia.com/mig.config.state' node label"
STATE=$(kubectl get node "${NODE_NAME}" -o=jsonpath='{.metadata.labels.nvidia\.com/mig\.config\.state}')
if [ "${?}" != "0" ]; then

	echo "Unable to get the value of the 'nvidia.com/mig.config.state' label"
	exit_failed

fi
echo "Current value of 'nvidia.com/mig.config.state=${STATE}'"
*/
func currentStateLabel(nodeName string) (string, error) {
	cmd := exec.Command("kubectl", "get", "node", nodeName, "-o=jsonpath={.metadata.labels.nvidia.com/mig.config.state}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Unable to get the value of the 'nvidia.com/mig.config.state' label")
	}
	state := strings.TrimSpace(string(output))
	return state, nil
}

/*
migConfigCurrentlyApplied
echo "Checking if the selected MIG config is currently applied or not"
nvidia-mig-parted assert -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
if [ "${?}" = "0" ]; then

	exit_success

fi
*/
func migConfigCurrentlyApplied(nvidiaMigPartedAlias string, migConfigFile string, selectedMIGConfig string) error {
	cmd := exec.Command(nvidiaMigPartedAlias, "assert", "-f", migConfigFile, "-c", selectedMIGConfig)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

/*
rightConfigApplied
if [ "${HOST_ROOT_MOUNT}" != "" ] && [ "${HOST_MIG_MANAGER_STATE_FILE}" != "" ]; then

	if [ -f "${HOST_ROOT_MOUNT}/${HOST_MIG_MANAGER_STATE_FILE}" ]; then
		echo "Persisting ${SELECTED_MIG_CONFIG} to ${HOST_MIG_MANAGER_STATE_FILE}"
		host_persist_config
		if [ "${?}" != "0" ]; then
			echo "Unable to persist ${SELECTED_MIG_CONFIG} to ${HOST_MIG_MANAGER_STATE_FILE}"
			exit_failed
		fi
	fi

fi
*/
func rightConfigApplied(hostRootMount string, hostMIGManagerStateFile string, selectedMIGConfig string, currentLabels map[string]string, hostGPUClientServices []string, nodeName string, withShutdownHostGPUClients bool) error {
	if hostRootMount == "" || hostMIGManagerStateFile == "" {
		return fmt.Errorf("Either container or host path is missing")
	}
	if _, err := os.Stat(hostRootMount + "/" + hostMIGManagerStateFile); err == nil {
		fmt.Printf("Persisting %v to %v \n", selectedMIGConfig, hostMIGManagerStateFile)
		if err := hostPersistConfig(selectedMIGConfig, hostRootMount, hostMIGManagerStateFile); err != nil {
			fmt.Printf("Unable to persist %v to %v \n", selectedMIGConfig, hostMIGManagerStateFile)
			setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		}
	}
	return nil
}

func hostPersistConfig(selectedMIGConfig string, hostRootMount string, hostMIGManagerStateFile string) error {
	config := `[Service]
	Environment="MIG_PARTED_SELECTED_CONFIG=` + selectedMIGConfig + `"`

	cmd := exec.Command("chroot", hostRootMount, "bash", "-c",
		fmt.Sprintf(`echo "%s" > %s; systemctl daemon-reload`, config, hostMIGManagerStateFile))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

/*
migModeSettingApplied
echo "Checking if the MIG mode setting in the selected config is currently applied or not"
echo "If the state is 'rebooting', we expect this to always return true"
nvidia-mig-parted assert --mode-only -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
if [ "${?}" != "0" ]; then

	if [ "${STATE}" = "rebooting" ]; then
		echo "MIG mode change did not take effect after rebooting"
		exit_failed
	fi
	if [ "${WITH_SHUTDOWN_HOST_GPU_CLIENTS}" = "true" ]; then
		HOST_GPU_CLIENT_SERVICES+=(${HOST_KUBELET_SERVICE})
	fi
	MIG_MODE_CHANGE_REQUIRED="true"

fi
*/
func migModeSettingApplied(nvidiaMigPartedAlias string, migConfigFile string, selectedMIGConfig string) error {
	fmt.Println("Checking if the MIG mode setting in the selected config is currently applied or not")
	fmt.Println("If the state is 'rebooting', we expect this to always return true")
	cmd := exec.Command(nvidiaMigPartedAlias, "assert", "--mode-only", "-f", migConfigFile, "-c", selectedMIGConfig)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mig Mode setting not applied")
	}
	return nil
}

/*
setStateToPending
echo "Changing the 'nvidia.com/mig.config.state' node label to 'pending'"

	kubectl label --overwrite  \
		node ${NODE_NAME} \
		nvidia.com/mig.config.state="pending"

if [ "${?}" != "0" ]; then

	echo "Unable to set the value of 'nvidia.com/mig.config.state' to 'pending'"
	exit_failed

fi
*/
func setStateToPending(nodeName string) error {
	fmt.Println("Changing the 'nvidia.com/mig.config.state' node label to 'pending'")
	cmd := exec.Command("kubectl", "label", "--overwrite", "node", nodeName, "nvidia.com/mig.config.state=pending")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Unable to set value to pending")
	}
	return nil
}

/**
shuttingGPUClients
echo "Shutting down all GPU clients in Kubernetes by disabling their component-specific nodeSelector labels"
kubectl label --overwrite \
	node ${NODE_NAME} \
	nvidia.com/gpu.deploy.device-plugin=$(maybe_set_paused ${PLUGIN_DEPLOYED}) \
	nvidia.com/gpu.deploy.gpu-feature-discovery=$(maybe_set_paused ${GFD_DEPLOYED}) \
	nvidia.com/gpu.deploy.dcgm-exporter=$(maybe_set_paused ${DCGM_EXPORTER_DEPLOYED}) \
	nvidia.com/gpu.deploy.dcgm=$(maybe_set_paused ${DCGM_DEPLOYED}) \
	nvidia.com/gpu.deploy.nvsm=$(maybe_set_paused ${NVSM_DEPLOYED})
if [ "${?}" != "0" ]; then
	echo "Unable to tear down GPU client pods by setting their daemonset labels"
	exit_failed
fi

echo "Waiting for the device-plugin to shutdown"
kubectl wait --for=delete pod \
	--timeout=5m \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n "${DEFAULT_GPU_CLIENTS_NAMESPACE}" \
	-l app=nvidia-device-plugin-daemonset

echo "Waiting for gpu-feature-discovery to shutdown"
kubectl wait --for=delete pod \
	--timeout=5m \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n "${DEFAULT_GPU_CLIENTS_NAMESPACE}" \
	-l app=gpu-feature-discovery

echo "Waiting for dcgm-exporter to shutdown"
kubectl wait --for=delete pod \
	--timeout=5m \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n "${DEFAULT_GPU_CLIENTS_NAMESPACE}" \
	-l app=nvidia-dcgm-exporter

echo "Waiting for dcgm to shutdown"
kubectl wait --for=delete pod \
	--timeout=5m \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n "${DEFAULT_GPU_CLIENTS_NAMESPACE}" \
	-l app=nvidia-dcgm

echo "Removing the cuda-validator pod"
kubectl delete pod \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n "${DEFAULT_GPU_CLIENTS_NAMESPACE}" \
	-l app=nvidia-cuda-validator

echo "Removing the plugin-validator pod"
kubectl delete pod \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n "${DEFAULT_GPU_CLIENTS_NAMESPACE}" \
	-l app=nvidia-device-plugin-validator

if [ "${WITH_SHUTDOWN_HOST_GPU_CLIENTS}" = "true" ]; then
	echo "Shutting down all GPU clients on the host by stopping their systemd services"
	host_stop_systemd_services
	if [ "${?}" != "0" ]; then
		echo "Unable to shutdown GPU clients on host by stopping their systemd services"
		exit_failed
	fi
	if [ "${MIG_MODE_CHANGE_REQUIRED}" = "true" ]; then
		# This is a hack to accommodate for observed behaviour. Once we shut
		# down the above services, there appears to be some settling time
		# before we are able to reconnect to the fabric-manager to run the
		# required GPU reset when changing MIG mode. It is unknown why this
		# problem only appears when shutting down systemd services on the host
		# with pre-installed drivers, and not when running with operator
		# managed drivers.
		sleep 30
	fi
fi
**/

func shuttingGPUClients(currentLabels map[string]string, nodeName string, defaultGPUClientsNamespace string, withShutdownHostGPUClients bool, hostRootMount string, hostGPUClientServices []string) error {
	// disabling lables
	label := []string{"kubectl label --overwrite node ${nodeName}"}
	for k, v := range currentLabels {
		state := MaybeSetPaused(v)
		key := fmt.Sprintf("nvidia.com/gpu.deploy.%s=%s", k, state)
		label = append(label, key)
	}
	cmd := strings.Join(label, " ")
	_, err := getCmdOutput(cmd)
	if err != nil {
		fmt.Println("Unable to tear down GPU client pods by setting their daemonset labels")
		setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
	}

	// shutting down pods & removing validator pods
	if err := shuttingDownPods(nodeName, defaultGPUClientsNamespace); err != nil {
		return err
	}

	// host_stop_systemd_services
	if withShutdownHostGPUClients {
		fmt.Println("Shutting down all GPU clients on the host by stopping their systemd services")
		if err := hostStopSystemdServices(hostRootMount, hostGPUClientServices); err != nil {
			fmt.Println("Unable to shutdown GPU clients on host by stopping their systemd services")
			setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		}
		if migModeChangeRequired {
			time.Sleep(30 * time.Second)
		}
	}
	return nil
}

func shuttingDownPods(nodeName string, defaultGPUClientsNamespace string) error {

	labels := []string{
		"nvidia-device-plugin-daemonset",
		"gpu-feature-discovery",
		"nvidia-dcgm-exporter",
		"nvidia-dcgm",
	}

	validators := []string{
		"nvidia-cuda-validator",
		"nvidia-device-plugin-validator",
	}

	for _, label := range labels {
		fmt.Printf("Waiting for the %s to shutdown\n", label)
		args := []string{"wait", "--for=delete", "pod", "--timeout=5m", "--field-selector",
			fmt.Sprintf("spec.nodeName=%s", nodeName),
			"-n", defaultGPUClientsNamespace,
			"-l", fmt.Sprintf("app=%s", label),
		}
		cmd := exec.Command("kubectl", args...)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("Unable to wait for %s to shutdown: %v", label, err)
		}
	}

	for _, validator := range validators {
		fmt.Printf("Removing the %s pod\n", validator)
		args := []string{"delete", "pod", "--field-selector",
			fmt.Sprintf("spec.nodeName=%s", nodeName),
			"-n", defaultGPUClientsNamespace,
			"-l", fmt.Sprintf("app=%s", validator),
		}
		cmd := exec.Command("kubectl", args...)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("Unable to remove %s pod: %v", validator, err)
		}
	}
	return nil
}

func hostStopSystemdServices(hostRootMount string, hostGPUClientServices []string) error {
	for _, s := range hostGPUClientServices {
		// If the service is "active"" we will attempt to shut it down and (if
		// successful) we will track it to restart it later.
		cmd := exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-active", s)
		err := cmd.Run()
		if err == nil {
			fmt.Printf("Stopping %s (active, will-restart)\n", s)
			cmd = exec.Command("chroot", hostRootMount, "systemctl", "stop", s)
			err = cmd.Run()
			if err != nil {
				return fmt.Errorf("failed to stop service %s: %w", s, err)
			}
			hostGPUClientServicesStopped = append([]string{s}, hostGPUClientServicesStopped...)
			continue
		}

		// If the service is inactive, then we may or may not still want to track
		// it to restart it later. The logic below decides when we should or not.
		cmd = exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-enabled", s)
		if err = cmd.Run(); err != nil {
			fmt.Printf("Skipping %s (no-exist)", s)
			continue
		}

		cmd = exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-failed", s)
		if err = cmd.Run(); err == nil {
			fmt.Printf("Skipping %s (is-failed, will-restart)\n", s)
			hostGPUClientServicesStopped = append([]string{s}, hostGPUClientServicesStopped...)
			continue
		}

		cmd = exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-enabled", s)
		if err = cmd.Run(); err != nil {
			fmt.Printf("Skipping %s (disabled)\n", s)
			continue
		}

		cmd = exec.Command("chroot", hostRootMount, "systemctl", "show", "--property=Type", s)
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to get service type for %s: %w", s, err)
		}
		if strings.TrimSpace(string(output)) == "Type=oneshot" {
			fmt.Printf("Skipping %s (inactive, oneshot, no-restart)\n", s)
			continue
		}

		fmt.Printf("Skipping %s (inactive, will-restart)", s)
		hostGPUClientServicesStopped = append([]string{s}, hostGPUClientServicesStopped...)
	}
	return nil
}

/*
applyMIGModeChange
echo "Applying the MIG mode change from the selected config to the node (and double checking it took effect)"
echo "If the -r option was passed, the node will be automatically rebooted if this is not successful"
nvidia-mig-parted -d apply --mode-only -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
nvidia-mig-parted -d assert --mode-only -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
if [ "${?}" != "0" ] && [ "${WITH_REBOOT}" = "true" ]; then

	echo "Changing the 'nvidia.com/mig.config.state' node label to 'rebooting'"
	kubectl label --overwrite  \
		node ${NODE_NAME} \
		nvidia.com/mig.config.state="rebooting"
	if [ "${?}" != "0" ]; then
		echo "Unable to set the value of 'nvidia.com/mig.config.state' to 'rebooting'"
		echo "Exiting so as not to reboot multiple times unexpectedly"
		exit_failed
	fi
	chroot ${HOST_ROOT_MOUNT} reboot
	exit 0

fi
*/
func applyMIGModeChange(nvidiaMigPartedAlias string, migConfigFile string, selectedMIGConfig string,
	 nodeName string, withReboot bool, hostRootMount string, currentLabels map[string]string, hostGPUClientServices []string, withShutdownHostGPUClients bool) error {
	applyCmd := exec.Command(nvidiaMigPartedAlias, "-d", "apply", "--mode-only", "-f", migConfigFile, "-c", selectedMIGConfig)
	assertCmd := exec.Command(nvidiaMigPartedAlias, "-d", "assert", "--mode-only", "-f", migConfigFile, "-c", selectedMIGConfig)

	if err := applyCmd.Run(); err != nil {
		fmt.Println("Failed to apply MIG mode change: ", err)
		if withReboot {
			rebootNode(nodeName, hostRootMount, currentLabels, hostGPUClientServices, withShutdownHostGPUClients)
		}
		return err
	}

	if err := assertCmd.Run(); err != nil {
		fmt.Println("Failed to assert MIG mode change: ", err)
		if withReboot {
			rebootNode(nodeName, hostRootMount, currentLabels, hostGPUClientServices, withShutdownHostGPUClients)
		}
		return err
	}
	fmt.Println("MIG mode change applied successfully")
	return nil
}

func rebootNode(nodeName string, hostRootMount string, currentLabels map[string]string, hostGPUClientServices []string, withShutdownHostGPUClients bool) {

	fmt.Println("Changing the 'nvidia.com/mig.config.state' node label to 'rebooting'")
	cmd := exec.Command("kubectl", "label", "--overwrite", "node", nodeName, "nvidia.com/mig.config.state=rebooting")
	if err := cmd.Run(); err != nil {
		fmt.Println("Unable to set the value of 'nvidia.com/mig.config.state' to 'rebooting'")
		fmt.Println("Exiting so as not to reboot multiple times unexpectedly")
		setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
	}

	cmdRoot := exec.Command("chroot", hostRootMount, "reboot")
	if err := cmdRoot.Run(); err != nil {
		fmt.Println("Failed to reboot the node: ", err)
		os.Exit(0)
	}
}

/*
	echo "Applying the selected MIG config to the node"
	nvidia-mig-parted -d apply -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
	if [ "${?}" != "0" ]; then
		exit_failed
	fi
*/

func applyMigConfig(nvidiaMigPartedAlias string, migConfigFile string, selectedMIGConfig string, currentLabels map[string]string, hostGPUClientServices []string, withShutdownHostGPUClients bool, hostRootMount string, nodeName string) error {
	fmt.Println("Applying the selected MIG config to the node")
	cmd := exec.Command(nvidiaMigPartedAlias, "-d", "apply", "-f", migConfigFile, "-c", selectedMIGConfig)
	if err := cmd.Run(); err != nil {
		setStateAndExit("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
	}
	return nil
}

/*
cdiEnabled
if [ "${CDI_ENABLED}" = "true" ]; then

	echo "Running nvidia-smi"
	chroot ${DRIVER_ROOT_CTR_PATH} nvidia-smi >/dev/null
	if [ "${?}" != "0" ]; then
		exit_failed
	fi

	echo "Creating NVIDIA control device nodes"
	nvidia-ctk system create-device-nodes --control-devices --driver-root=${DRIVER_ROOT_CTR_PATH}
	if [ "${?}" != "0" ]; then
		exit_failed
	fi

	echo "Creating management CDI spec"
	nvidia-ctk cdi generate --mode=management \
		--driver-root=${DRIVER_ROOT_CTR_PATH} \
		--vendor="management.nvidia.com" \
		--class="gpu" \
		--nvidia-ctk-path="/usr/local/nvidia/toolkit/nvidia-ctk" | \
			nvidia-ctk cdi transform root \
				--from=$DRIVER_ROOT_CTR_PATH \
				--to=$DRIVER_ROOT \
				--input="-" \
				--output="/var/run/cdi/management.nvidia.com-gpu.yaml"
	if [ "${?}" != "0" ]; then
		exit_failed
	fi

fi
*/
func cdiEnabled(CDIEnabled bool, driverRootCTRPath string, driverRoot string) error {
	if CDIEnabled {
		fmt.Println("Running nvidia-smi")
		_, err := chroot(driverRootCTRPath, "nvidia-smi")
		if err != nil {
			return err
		}
	}

	fmt.Println("Creating NVIDIA control device nodes")
	_, err := nvidia_ctk("system", "create-device-nodes", "--control-devices", fmt.Sprintf("--driver-root=%s", driverRootCTRPath))
	if err != nil {
		return err
	}

	fmt.Println("Creating management CDI spec")
	_, err = nvidia_ctk("cdi", "generate", "--mode=management",
		fmt.Sprintf("--driver-root=%s", driverRootCTRPath),
		"--vendor=management.nvidia.com",
		"--class=gpu",
		"--nvidia-ctk-path=/usr/local/nvidia/toolkit/nvidia-ctk")
	if err != nil {
		return err
	}

	_, err = nvidia_ctk("cdi", "transform", "root",
		fmt.Sprintf("--from=%s", driverRootCTRPath),
		fmt.Sprintf("--to=%s", driverRoot),
		"--input=-",
		"--output=/var/run/cdi/management.nvidia.com-gpu.yaml")
	if err != nil {
		return err
	}
	return nil
}

func chroot(rootPath string, command string) ([]byte, error) {
	cmd := exec.Command("chroot", rootPath, command)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

func nvidia_ctk(args ...string) ([]byte, error) {
	cmd := exec.Command("nvidia-ctk", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

/*
	if [ "${WITH_SHUTDOWN_HOST_GPU_CLIENTS}" = "true" ]; then
		echo "Restarting all GPU clients previously shutdown on the host by restarting their systemd services"
		NO_RESTART_HOST_SYSTEMD_SERVICES_ON_EXIT=true
		host_start_systemd_services
		if [ "${?}" != "0" ]; then
			echo "Unable to restart GPU clients on host by restarting their systemd services"
			exit_failed
		fi
	fi

	echo "Restarting validator pod to re-run all validations"
	kubectl delete pod \
		--field-selector "spec.nodeName=${NODE_NAME}" \
		-n "${DEFAULT_GPU_CLIENTS_NAMESPACE}" \
		-l app=nvidia-operator-validator

	echo "Restarting all GPU clients previously shutdown in Kubernetes by reenabling their component-specific nodeSelector labels"
	NO_RESTART_K8S_DAEMONSETS_ON_EXIT=true
	kubectl label --overwrite \
		node ${NODE_NAME} \
		nvidia.com/gpu.deploy.device-plugin=$(maybe_set_true ${PLUGIN_DEPLOYED}) \
		nvidia.com/gpu.deploy.gpu-feature-discovery=$(maybe_set_true ${GFD_DEPLOYED}) \
		nvidia.com/gpu.deploy.dcgm-exporter=$(maybe_set_true ${DCGM_EXPORTER_DEPLOYED}) \
		nvidia.com/gpu.deploy.dcgm=$(maybe_set_true ${DCGM_DEPLOYED}) \
		nvidia.com/gpu.deploy.nvsm=$(maybe_set_true ${NVSM_DEPLOYED})
	if [ "${?}" != "0" ]; then
		echo "Unable to bring up GPU client components by setting their daemonset labels"
		exit_failed
	fi
*/

func restartingGPUClients(currentLabels map[string]string, withShutdownHostGPUClients bool, defaultGPUClientsNamespace string, hostRootMount string, hostGPUClientServices []string, nodeName string) error {

	if withShutdownHostGPUClients {
		fmt.Println("Restarting all GPU clients previously shutdown on the host by restarting their systemd services")
		noRestartHostSystemdServicesOnExit = true
		err := hostStartSystemdServices(hostRootMount, hostGPUClientServices)
		if err != nil {
			return fmt.Errorf("Unable to restart GPU clients on host by restarting their systemd services")
		}
	}

	// before k8 host_start_systemd_services
	label := []string{"kubectl label --overwrite node ${NODE_NAME}"}
	for k, v := range currentLabels {
		state := MaybeSetTrue(v)
		key := fmt.Sprintf("kubectl nvidia.com/gpu.deploy.%s=%s", k, state)
		label = append(label, key)
	}
	cmd := strings.Join(label, " ")
	_, err := getCmdOutput(cmd)
	if err != nil {
		return err
	}

	fmt.Println("Restarting validator pod to re-run all validations")
	run := exec.Command("kubectl","delete", "pod", "--field-selector",
		fmt.Sprintf("spec.nodeName=%s", nodeName),
		"-n", defaultGPUClientsNamespace,
		"-l", "app=nvidia-operator-validator")
	run.Stdout = os.Stdout
	run.Stderr = os.Stderr
	if err = run.Run(); err != nil {
		return fmt.Errorf("error restarting validator pod: %s", err)
	}
	return nil
}

func hostStartSystemdServices(hostRootMount string, hostGPUClientServices []string) error {
	// If HOST_GPU_CLIENT_SERVICES_STOPPED is empty, then it's possible that
	// host_stop_systemd_services was never called, so let's double check to see
	// if there's anything we should actually restart.
	if len(hostGPUClientServicesStopped) == 0 {
		for _, s := range hostGPUClientServices {
			out, err := exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-active", s).CombinedOutput()
			if err == nil {
				continue
			}

			out, err = exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-enabled", s).CombinedOutput()
			if err != nil {
				continue
			}

			out, err = exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-failed", s).CombinedOutput()
			if err == nil {
				hostGPUClientServicesStopped = append([]string{s}, hostGPUClientServicesStopped...)
			}

			out, err = exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-enabled", s).CombinedOutput()
			if err != nil {
				continue
			}

			out, err = exec.Command("chroot", hostRootMount, "systemctl", "show", "--property=Type", s).CombinedOutput()
			if err == nil && strings.TrimSpace(string(out)) == "Type=oneshot" {
				continue
			}

			hostGPUClientServicesStopped = append([]string{s}, hostGPUClientServicesStopped...)
		}
	}

	for _, s := range hostGPUClientServicesStopped {
		fmt.Printf("Starting %s\n", s)
		_, err := exec.Command("chroot", hostRootMount, "systemctl", "start", s).CombinedOutput()
		if err != nil {
			fmt.Printf("Error Starting %s: skipping, but continuing...", s)
			return err
		}
	}

	return nil
}
