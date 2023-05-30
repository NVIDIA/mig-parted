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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

var (
	hostGPUClientServicesStopped            = []string{}
	migModeChangeRequired              bool = true
	noRestartHostSystemdServicesOnExit bool
	noRestartK8sDaemonsetsOnExit       bool
)

type migConfig struct {
	migParted     string
	migConfigFile string
}

// reconfigure function applies the User's selected configuration to the node using Mig-Parted tool
func reconfigure(nodeName string, migConfigFile string, selectedMIGConfig string, hostRootMount string, hostNvidiaDir string, hostMIGManagerStateFile string, hostGPUClientServices []string, hostKubeletServices string,
	defaultGPUClientsNamespace string, CDIEnabled bool, driverRoot string, driverRootCTRPath string, withReboot bool, withShutdownHostGPUClients bool) error {

	klog.InitFlags(nil)
	defer klog.Flush()
	flag.Parse()

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

	// Declaring and initializing a struct using a struct literal
	migConfigProperties := migConfig{nvidiaMigPartedAlias, migConfigFile}

	/*
		retrieves current values of kubernetes node labels
		i/p: nodeName, (device-plugin, gpu-feature-discovery, dcgm-exporter, dcgm, nvsm)
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
	if err := assertConfigPresent(migConfigProperties, selectedMIGConfig); err != nil {
		setState("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return err
	}

	/*
		capturing current value of the mig.config.state label
		i/p: nodeName
		o/p: state
	*/
	state, err := currentStateLabel(nodeName)
	if err != nil {
		setState("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return err
	}

	/*
		checking if selected MIG config is currently applied or not
		i/p: nvidiaMigPartedAlias, migConfigFile, selectedMIGConfig
		o/p: True or false
	*/
	if isMigConfigCurrentlyApplied(migConfigProperties, selectedMIGConfig) == true {
		setState("success", 0, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return nil
	}

	/*
		TODO: rightConfigApplied
		ensuring that right config is applied to the node if the MIG Parted is also installed as a systemd service
		i/p: hostRootMount, hostMIGManagerStateFile, selectedMIGConfig, currentLabels, hostGPUClientServices, nodeName, withShutdownHostGPUClients
		o/p: true or false

		if err := rightConfigApplied(hostRootMount, hostMIGManagerStateFile, selectedMIGConfig, currentLabels, hostGPUClientServices, nodeName, withShutdownHostGPUClients); err != nil{
			return err
		}
	*/

	/*
		Checking if the MIG mode setting in the selected config is currently applied or not
		i/p: migConfigFile, selectedMIGConfig, STATE, withShutdownHostGPUClients, hostGPUClientServices,
		     hostKubeletServices, migModeChangeRequired
		o/p: migModeChangeRequired
	*/
	if err := migModeSettingApplied(migConfigProperties, selectedMIGConfig); err != nil {
		if state == "rebooting" {
			fmt.Println("MIG mode change did not take effect after rebooting")
			setState("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
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
	if err := setStateToX(nodeName, "pending"); err != nil {
		fmt.Println("Unable to set the value of 'nvidia.com/mig.config.state' to 'pending'")
		setState("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
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
	if err := applyMIGModeChange(migConfigProperties, selectedMIGConfig, nodeName, withReboot, hostRootMount, currentLabels, hostGPUClientServices, withShutdownHostGPUClients); err != nil {
		return err
	}

	/*
		applying the selected configuration on the node
		i/p: migConfigFile, selectedMIGConfig
		o/p: true or false (error)
	*/
	if err := applyMigConfig(migConfigProperties, selectedMIGConfig, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName); err != nil {
		setState("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return err
	}

	/*
		on exit or at the end
		restarting all GPU clients (systemd services, component-specific nodeSelector)
		i/p: withShutdownHostGPUClients, noRestartHostSystemdServicesOnExit, nodeName,
			defaultGPUClientsNamespace, noRestartK8sDaemonsetsOnExit, currentLabels
	*/
	if err := restartingGPUClients(currentLabels, withShutdownHostGPUClients, defaultGPUClientsNamespace, hostRootMount, hostGPUClientServices, nodeName); err != nil {
		setState("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return err
	}

	setState("success", 0, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
	return nil
}

func setState(state string, exitCode int, currentLabels map[string]string, hostGPUClientServices []string, withShutdownHostGPUClients bool, hostRootMount string, nodeName string) {

	if withShutdownHostGPUClients == true {
		if noRestartHostSystemdServicesOnExit != true {
			fmt.Println("Restarting any GPU clients previously shutdown on the host by restarting their systemd services")
			ret := hostStartSystemdServices(hostRootMount, hostGPUClientServices)
			if ret != nil {
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
			"nvidia.com/gpu.deploy.device-plugin=" + maybeSetTrue(currentLabels["device-plugin"]),
			"nvidia.com/gpu.deploy.gpu-feature-discovery=" + maybeSetTrue(currentLabels["gpu-feature-discovery"]),
			"nvidia.com/gpu.deploy.dcgm-exporter=" + maybeSetTrue(currentLabels["dcgm-exporter"]),
			"nvidia.com/gpu.deploy.dcgm=" + maybeSetTrue(currentLabels["dcgm"]),
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

// Getting current value of the 'nvidia.com/gpu.deploy.__' node label,
//
//	and saving values in a map "currentLabels"
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

// Asserting that the requested configuration is present in the configuration file
func assertConfigPresent(migConfigProperties migConfig, selectedMIGConfig string) error {
	_, err := getCmdOutput(fmt.Sprintf("%s assert --valid-config -f %s -c %s", migConfigProperties.migParted, migConfigProperties.migConfigFile, selectedMIGConfig))
	if err != nil {
		return fmt.Errorf("Unable to validate the selected MIG configuration: %w", err)
	}
	return nil
}

// Getting current value of the 'nvidia.com/mig.config.state' node label
func currentStateLabel(nodeName string) (string, error) {
	cmd := exec.Command("kubectl", "get", "node", nodeName, "-o=jsonpath={.metadata.labels.nvidia.com/mig.config.state}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("Unable to get the value of the 'nvidia.com/mig.config.state' label")
	}
	state := strings.TrimSpace(string(output))
	return state, nil
}

// Checking if the selected MIG config is currently applied or not
func isMigConfigCurrentlyApplied(migConfigProperties migConfig, selectedMIGConfig string) bool {
	cmd := exec.Command(migConfigProperties.migParted, "assert", "-f", migConfigProperties.migConfigFile, "-c", selectedMIGConfig)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func rightConfigApplied(hostRootMount string, hostMIGManagerStateFile string, selectedMIGConfig string, currentLabels map[string]string, hostGPUClientServices []string, nodeName string, withShutdownHostGPUClients bool) error {
	if hostRootMount == "" || hostMIGManagerStateFile == "" {
		return fmt.Errorf("Either container or host path is missing")
	}
	if _, err := os.Stat(hostRootMount + "/" + hostMIGManagerStateFile); err == nil {
		fmt.Printf("Persisting %v to %v \n", selectedMIGConfig, hostMIGManagerStateFile)
		if err := hostPersistConfig(selectedMIGConfig, hostRootMount, hostMIGManagerStateFile); err != nil {
			fmt.Printf("Unable to persist %v to %v \n", selectedMIGConfig, hostMIGManagerStateFile)
			setState("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
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

func migModeSettingApplied(migConfigProperties migConfig, selectedMIGConfig string) error {
	fmt.Println("Checking if the MIG mode setting in the selected config is currently applied or not")
	fmt.Println("If the state is 'rebooting', we expect this to always return true")
	cmd := exec.Command(migConfigProperties.migParted, "assert", "--mode-only", "-f", migConfigProperties.migConfigFile, "-c", selectedMIGConfig)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mig Mode setting not applied")
	}
	return nil
}

// Setting mig.config.state label to the provided value (rebooting, pending..)
func setStateToX(nodeName string, s string) error {
	fmt.Printf("Changing the 'nvidia.com/mig.config.state' node label to '%s'\n", s)
	cmd := exec.Command("kubectl", "label", "--overwrite", "node", nodeName, "nvidia.com/mig.config.state=%s", s)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Unable to set value to %s", s)
	}
	return nil
}

// Shutting down all GPU clients in Kubernetes by disabling their component-specific nodeSelector labels
func shuttingGPUClients(currentLabels map[string]string, nodeName string, defaultGPUClientsNamespace string, withShutdownHostGPUClients bool, hostRootMount string, hostGPUClientServices []string) error {
	// disabling lables
	label := []string{"kubectl label --overwrite node ${nodeName}"}
	for k, v := range currentLabels {
		state := maybeSetPaused(v)
		key := fmt.Sprintf("nvidia.com/gpu.deploy.%s=%s", k, state)
		label = append(label, key)
	}
	cmd := strings.Join(label, " ")
	_, err := getCmdOutput(cmd)
	if err != nil {
		fmt.Println("Unable to tear down GPU client pods by setting their daemonset labels")
		setState("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
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
			setState("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
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

func hostStopSystemdServicesInactive(hostRootMount string, hostGPUClientServices []string, hostGPUClientServicesStopped []string, s string) {
	// If the service is inactive, then we may or may not still want to track
	// it to restart it later. The logic below decides when we should or not.
	cmd := exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-enabled", s)
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Skipping %s (no-exist)\n", s)
		return
	}

	cmd = exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-failed", s)
	if err = cmd.Run(); err == nil {
		fmt.Printf("Skipping %s (is-failed, will-restart)\n", s)
		hostGPUClientServicesStopped = append([]string{s}, hostGPUClientServicesStopped...)
		return
	}

	cmd = exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-enabled", s)
	if err = cmd.Run(); err != nil {
		fmt.Printf("Skipping %s (disabled)\n", s)
		return
	}

	cmd = exec.Command("chroot", hostRootMount, "systemctl", "show", "--property=Type", s)
	output, _ := cmd.Output()
	if strings.TrimSpace(string(output)) == "Type=oneshot" {
		fmt.Printf("Skipping %s (inactive, oneshot, no-restart)\n", s)
		return
	}

	fmt.Printf("Skipping %s (inactive, will-restart)", s)
	hostGPUClientServicesStopped = append([]string{s}, hostGPUClientServicesStopped...)
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

		hostStopSystemdServicesInactive(hostRootMount, hostGPUClientServices, hostGPUClientServicesStopped, s)
	}
	return nil
}

// Applying the MIG mode change from the selected config to the node (and double checking it took effect)
func applyMIGModeChange(migConfigProperties migConfig, selectedMIGConfig string,
	nodeName string, withReboot bool, hostRootMount string, currentLabels map[string]string, hostGPUClientServices []string, withShutdownHostGPUClients bool) error {

	applyCmd := exec.Command(migConfigProperties.migParted, "-d", "apply", "--mode-only", "-f", migConfigProperties.migConfigFile, "-c", selectedMIGConfig)
	if err := applyCmd.Run(); err != nil {
		fmt.Println("Failed to apply MIG mode change: ", err)
	}

	if err := migModeSettingApplied(migConfigProperties, selectedMIGConfig); err != nil {
		fmt.Println("Failed to assert MIG mode change: ", err)
		if withReboot {
			rebootNode(nodeName, hostRootMount, currentLabels, hostGPUClientServices, withShutdownHostGPUClients)
		}
		return err
	}
	klog.Info("MIG mode change applied successfully")
	return nil
}

func rebootNode(nodeName string, hostRootMount string, currentLabels map[string]string, hostGPUClientServices []string, withShutdownHostGPUClients bool) {

	fmt.Println("Changing the 'nvidia.com/mig.config.state' node label to 'rebooting'")

	if err := setStateToX(nodeName, "rebooting"); err != nil {
		fmt.Println("Unable to set the value of 'nvidia.com/mig.config.state' to 'rebooting'")
		fmt.Println("Exiting so as not to reboot multiple times unexpectedly")
		setState("failed", 1, currentLabels, hostGPUClientServices, withShutdownHostGPUClients, hostRootMount, nodeName)
		return
	}

	cmdRoot := exec.Command("chroot", hostRootMount, "reboot")
	if err := cmdRoot.Run(); err != nil {
		fmt.Println("Failed to reboot the node: ", err)
		os.Exit(0)
	}
}

func applyMigConfig(migConfigProperties migConfig, selectedMIGConfig string, currentLabels map[string]string, hostGPUClientServices []string, withShutdownHostGPUClients bool, hostRootMount string, nodeName string) error {
	fmt.Println("Applying the selected MIG config to the node")
	cmd := exec.Command(migConfigProperties.migParted, "-d", "apply", "-f", migConfigProperties.migConfigFile, "-c", selectedMIGConfig)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// Restarting Host GPU clients
// Restarting kubernetes GPU clients
// Restarting validator pods, to re-run all the validations
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
	fmt.Println("Restarting all GPU clients previously shutdown in Kubernetes by reenabling their component-specific nodeSelector labels")
	noRestartK8sDaemonsetsOnExit = true
	label := []string{"kubectl label --overwrite node ${NODE_NAME}"}
	for k, v := range currentLabels {
		state := maybeSetTrue(v)
		key := fmt.Sprintf("kubectl nvidia.com/gpu.deploy.%s=%s", k, state)
		label = append(label, key)
	}
	cmd := strings.Join(label, " ")
	_, err := getCmdOutput(cmd)
	if err != nil {
		return err
	}

	fmt.Println("Restarting validator pod to re-run all validations")
	run := exec.Command("kubectl", "delete", "pod", "--field-selector",
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
			_, err := exec.Command("chroot", hostRootMount, "systemctl", "-q", "is-active", s).CombinedOutput()
			if err == nil {
				continue
			}

			hostStopSystemdServicesInactive(hostRootMount, hostGPUClientServices, hostGPUClientServicesStopped, s)
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
