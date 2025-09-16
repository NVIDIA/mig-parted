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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	configStatePending   = "pending"
	configStateRebooting = "rebooting"

	ldPreloadEnvVar = "LD_PRELOAD"
)

type reconfigurer struct {
	*reconfigureMIGOptions
	commandRunner
	migParted migParted
	node      nodeLabeller
}

// A commandWithOutput runs a command and ensures that STDERR and STDOUT are
// set.
type commandWithOutput struct{}

var _ commandRunner = (*commandWithOutput)(nil)

// New creates a MIG Reconfigurer with the supplied options.
func New(opts ...Option) (Reconfigurer, error) {
	o := &reconfigureMIGOptions{}

	for _, opt := range opts {
		opt(o)
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	if o.CDIEnabled && o.DriverLibraryPath == "" {
		return nil, fmt.Errorf("A driver library path must be specified for CDI mode")
	}

	migParted, err := o.createMIGPartedCLI()
	if err != nil {
		return nil, err
	}

	r := &reconfigurer{
		reconfigureMIGOptions: o,
		commandRunner:         &commandWithOutput{},
		migParted:             migParted,
		node: &node{
			clientset: o.clientset,
			name:      o.NodeName,
		},
	}

	return r, nil
}

func (opts *reconfigureMIGOptions) createMIGPartedCLI() (*migPartedCLI, error) {
	c := &commandWithOutput{}

	if !opts.WithShutdownHostGPUClients {
		m := &migPartedCLI{
			path:                "/usr/bin/nvidia-mig-parted",
			MIGPartedConfigFile: opts.MIGPartedConfigFile,
			SelectedMIGConfig:   opts.SelectedMIGConfig,
			DriverLibraryPath:   opts.DriverLibraryPath,
			commandRunner:       c,
		}
		return m, nil
	}

	if opts.hostNVIDIADir == "" {
		return nil, fmt.Errorf("HOST_NVIDIA_DIR must be specified")
	}

	hostRoot, err := os.OpenRoot(opts.HostRootMount)
	if err != nil {
		return nil, fmt.Errorf("failed to open host root: %w", err)
	}
	defer hostRoot.Close()

	hostNVIDIADir := strings.TrimPrefix(opts.hostNVIDIADir, "/")
	// TODO: Once we switch to go 1.25, we can use os.Root.MkdirAll.
	// if err := hostRoot.Mkdir(hostNVIDIADir, 0755); err != nil {
	// 	return nil, fmt.Errorf("failed to create directory: %w", err)
	// }
	// if err := hostRoot.Mkdir(filepath.Join(hostNVIDIADir, "mig-manager"), 0755); err != nil {
	// 	return nil, fmt.Errorf("failed to create directory: %w", err)
	// }
	if err := os.MkdirAll(filepath.Join(opts.HostRootMount, opts.hostNVIDIADir, "mig-manager"), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	migParted, err := os.Open("/usr/bin/nvidia-mig-parted")
	if err != nil {
		return nil, err
	}
	defer migParted.Close()

	hostMigPartedPath := filepath.Join(hostNVIDIADir, "mig-manager", "nvidia-mig-parted")
	hostMigParted, err := hostRoot.Create(hostMigPartedPath)
	if err != nil {
		return nil, err
	}
	defer hostMigParted.Close()

	if _, err := io.Copy(hostMigParted, migParted); err != nil {
		return nil, err
	}
	// Ensure that the file is executable.
	if err := hostMigParted.Chmod(0755); err != nil {
		return nil, err
	}

	configFile, err := os.Open(opts.MIGPartedConfigFile)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()

	hostConfigFilePath := filepath.Join(hostNVIDIADir, "mig-manager", "config.yaml")
	hostConfigFile, err := hostRoot.Create(hostConfigFilePath)
	if err != nil {
		return nil, err
	}
	defer hostConfigFile.Close()

	if _, err := io.Copy(hostConfigFile, configFile); err != nil {
		return nil, err
	}

	m := &migPartedCLI{
		root:                hostRoot.Name(),
		path:                "/" + hostMigPartedPath,
		MIGPartedConfigFile: "/" + hostConfigFilePath,
		SelectedMIGConfig:   opts.SelectedMIGConfig,
		// TODO: We may need to update this for the host.
		DriverLibraryPath: "",
		commandRunner:     c,
	}

	return m, nil
}

// Reconfigure configures MIG (Multi-Instance GPU) settings on a Kubernetes
// node. It validates the requested configuration, checks the current state,
// applies MIG mode changes, manages host GPU client services, and handles
// reboots when necessary. The function ensures that MIG configurations are
// applied safely with proper service lifecycle management.
func (opts *reconfigurer) Reconfigure() (rerr error) {
	restartSystemdClients := true
	var systemdClients gpuClients

	restartK8sGPUClients := true
	k8sGPUClients := opts.node.getK8sGPUClients(opts.GPUClientNamespace)

	defer func() {
		// TODO(elezar): Check whether the systemd clients need to be restarted.
		// If we're returning due to an error in restarting the services, then
		// don't restart them here.
		if restartSystemdClients {
			if err := opts.hostStartSystemdServices(systemdClients); err != nil {
				rerr = errors.Join(rerr, err)
			}

		}
		// TODO(elezar): Check whether the k8s clients need to be restarted.
		// If we're returning due to an error in restartig the k8s clients, then
		// don't restart them here.
		if restartK8sGPUClients {
			if err := k8sGPUClients.Restart(); err != nil {
				rerr = errors.Join(rerr, err)
			}
		}
		// TODO(elezar): If we are not returning from a reboot, we should set
		// set the mig-state label to `failed` or `success` based on the value
		// of rerr
		if rerr != nil {
			_ = opts.node.setNodeLabelValue(opts.ConfigStateLabel, "failed")
		} else {
			_ = opts.node.setNodeLabelValue(opts.ConfigStateLabel, "success")
		}
	}()

	log.Info("Asserting that the requested configuration is present in the configuration file")
	if err := opts.migParted.assertValidMIGConfig(); err != nil {
		return fmt.Errorf("error validating the selected MIG configuration: %w", err)
	}

	log.Infof("Getting current value of the '%s' node label", opts.ConfigStateLabel)
	state, err := opts.node.getNodeLabelValue(opts.ConfigStateLabel)
	if err != nil {
		return fmt.Errorf("unable to get the value of the %q label: %w", opts.ConfigStateLabel, err)
	}
	log.Infof("Current value of '%s=%s'", opts.ConfigStateLabel, state)

	log.Info("Checking if the selected MIG config is currently applied or not")
	if err := opts.migParted.assertMIGConfig(); err == nil {
		log.Info("MIG configuration already applied")
		return nil
	}

	if opts.HostRootMount != "" && opts.HostMIGManagerStateFile != "" {
		stateFilePath := filepath.Join(opts.HostRootMount, opts.HostMIGManagerStateFile)
		if _, err := os.Stat(stateFilePath); err == nil {
			log.Infof("Persisting %s to %s", opts.SelectedMIGConfig, opts.HostMIGManagerStateFile)
			if err := opts.hostPersistConfig(); err != nil {
				return fmt.Errorf("unable to persist %s to %s: %w", opts.SelectedMIGConfig, opts.HostMIGManagerStateFile, err)
			}
		}
	}

	log.Info("Checking if the MIG mode setting in the selected config is currently applied or not")
	log.Infof("If the state is '%s', we expect this to always return true", configStateRebooting)
	migModeChangeRequired := false
	if err := opts.migParted.assertMIGModeOnly(); err != nil {
		if state == configStateRebooting {
			return fmt.Errorf("MIG mode change failed after reboot: %w", err)
		}
		if opts.WithShutdownHostGPUClients {
			opts.HostGPUClientServices = append(opts.HostGPUClientServices, opts.HostKubeletService)
		}
		migModeChangeRequired = true
	}

	log.Infof("Changing the %q node label to %q", opts.ConfigStateLabel, configStatePending)
	if err := opts.node.setNodeLabelValue(opts.ConfigStateLabel, configStatePending); err != nil {
		return fmt.Errorf("unable to set the value of %q to %q: %w", opts.ConfigStateLabel, configStatePending, err)
	}

	log.Infof("Shutting down all GPU clients in Kubernetes by disabling their component-specific nodeSelector labels")
	if err := k8sGPUClients.Stop(); err != nil {
		// TODO: Update this error message.
		return fmt.Errorf("unable to tear down GPU client pods by setting their daemonset labels: %w", err)
	}

	if opts.WithShutdownHostGPUClients {
		log.Info("Shutting down all GPU clients on the host by stopping their systemd services")
		if err := opts.hostStopSystemdServices(systemdClients); err != nil {
			return fmt.Errorf("unable to shutdown host GPU clients: %w", err)
		}
		if migModeChangeRequired {
			log.Info("Waiting 30 seconds for services to settle")
			time.Sleep(30 * time.Second)
		}
	}

	log.Info("Applying the MIG mode change from the selected config to the node (and double checking it took effect)")
	log.Info("If the -r option was passed, the node will be automatically rebooted if this is not successful")
	if err := opts.migParted.applyMIGModeOnly(); err != nil || opts.migParted.assertMIGModeOnly() != nil {
		if opts.WithReboot {
			log.Infof("Changing the '%s' node label to '%s'", opts.ConfigStateLabel, configStateRebooting)
			if err := opts.node.setNodeLabelValue(opts.ConfigStateLabel, configStateRebooting); err != nil {
				log.Errorf("Unable to set the value of '%s' to '%s'", opts.ConfigStateLabel, configStateRebooting)
				log.Error("Exiting so as not to reboot multiple times unexpectedly")
				return fmt.Errorf("unable to set the value of %q to %q: %w", opts.ConfigStateLabel, configStateRebooting, err)
			}
			return rebootHost(opts.HostRootMount)
		}
	}

	log.Info("Applying the selected MIG config to the node")
	if err := opts.migParted.applyMIGConfig(); err != nil {
		return err
	}

	if opts.CDIEnabled {
		// Run nvidia-smi to ensure that the kernel modules are loaded and the
		// basic device nodes are available.
		if err := opts.runNvidiaSMI(); err != nil {
			return err
		}

		// Create additional control devices that are not created by nvidia-smi
		// e.g. /dev/nvidia-uvm and /dev/nvidia-uvm-tools
		if err := opts.createControlDeviceNodes(); err != nil {
			return err
		}

		// Ensure that we regenerate a CDI spec for management containers.
		if err := opts.regenerateManagementCDISpec(); err != nil {
			return err
		}
	}

	if opts.WithShutdownHostGPUClients {
		log.Info("Restarting all GPU clients previously shutdown on the host by restarting their systemd services")
		if err := opts.hostStartSystemdServices(systemdClients); err != nil {
			restartSystemdClients = false
			return fmt.Errorf("unable to restart host GPU clients: %w", err)
		}
	}

	log.Info("Restarting validator pod to re-run all validations")

	// TODO: This cast here should not be needed.
	if err := withNoStop((opts.node.(*node).newPod(opts.GPUClientNamespace, "nvidia-operator-validator"))).Restart(); err != nil {
		klog.ErrorS(err, "could not restart nvidia-operator-validator")
	}

	log.Info("Restarting all GPU clients previously shutdown in Kubernetes by reenabling their component-specific nodeSelector labels")
	if err := k8sGPUClients.Restart(); err != nil {
		restartK8sGPUClients = false
		return fmt.Errorf("unable to bring up GPU client components by setting their daemonset labels: %w", err)
	}

	return nil
}

type migPartedCLI struct {
	root                string
	path                string
	MIGPartedConfigFile string
	SelectedMIGConfig   string
	DriverLibraryPath   string
	commandRunner
}

var _ migParted = (*migPartedCLI)(nil)

func (opts *migPartedCLI) assertValidMIGConfig() error {
	args := []string{
		"--debug",
		"assert",
		"--valid-config",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	return opts.runMigParted(args...)
}

func (opts *migPartedCLI) assertMIGConfig() error {
	args := []string{
		"--debug",
		"assert",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	return opts.runMigParted(args...)
}

func (opts *migPartedCLI) assertMIGModeOnly() error {
	args := []string{
		"--debug",
		"assert",
		"--mode-only",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	return opts.runMigParted(args...)
}

func (opts *migPartedCLI) applyMIGModeOnly() error {
	args := []string{
		"--debug",
		"apply",
		"--mode-only",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	return opts.runMigParted(args...)
}

func (opts *migPartedCLI) applyMIGConfig() error {
	args := []string{
		"--debug",
		"apply",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	return opts.runMigParted(args...)
}

func (opts *reconfigurer) hostPersistConfig() error {
	config := fmt.Sprintf(`[Service]
Environment="MIG_PARTED_SELECTED_CONFIG=%s"
`, opts.SelectedMIGConfig)

	stateFilePath := filepath.Join(opts.HostRootMount, opts.HostMIGManagerStateFile)
	// #nosec G306 -- We cannot use 0600 here as the file is read by systemd.
	if err := os.WriteFile(stateFilePath, []byte(config), 0644); err != nil {
		return err
	}

	cmd := exec.Command("chroot", opts.HostRootMount, "systemctl", "daemon-reload") // #nosec G204 -- HostRootMount is validated via dirpath validator.
	return opts.Run(cmd)
}

func (opts *reconfigureMIGOptions) hostStopSystemdServices(systemdGPUClients gpuClients) error {
	for _, serviceName := range opts.HostGPUClientServices {
		service := opts.newSystemdService(serviceName)

		mustRestart, err := service.Pause()
		if err != nil {
			return err
		}
		if mustRestart {
			systemdGPUClients = append(systemdGPUClients, service)
		}
	}
	return nil
}

func (opts *reconfigureMIGOptions) hostStartSystemdServices(systemdGPUClients gpuClients) error {
	if len(systemdGPUClients) == 0 {
		for _, serviceName := range opts.HostGPUClientServices {
			service := opts.newSystemdService(serviceName)

			if mustRestart, _ := service.shouldRestart(); mustRestart {
				systemdGPUClients = append(systemdGPUClients, service)
			}
		}
	}

	// TODO: We should allow restarts to continue on failure.
	if err := systemdGPUClients.Restart(); err != nil {
		return fmt.Errorf("some services failed to start: %w", err)
	}
	return nil
}

func (c *commandWithOutput) Run(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (opts *migPartedCLI) runMigParted(args ...string) error {
	cmd := opts.migPartedCmd(args...)
	return opts.Run(cmd)
}

func (opts *migPartedCLI) migPartedCmd(args ...string) *exec.Cmd {
	var commandAndArgs []string

	if opts.root != "" && opts.root != "/" {
		commandAndArgs = append(commandAndArgs, "chroot", opts.root)
	}
	commandAndArgs = append(commandAndArgs, opts.path)
	commandAndArgs = append(commandAndArgs, args...)

	cmd := exec.Command(commandAndArgs[0], commandAndArgs[1:]...) //nolint:gosec
	if opts.DriverLibraryPath != "" {
		cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", ldPreloadEnvVar, opts.DriverLibraryPath))
	}

	return cmd
}

func rebootHost(hostRootMount string) error {
	cmd := exec.Command("chroot", hostRootMount, "reboot")
	if err := cmd.Start(); err != nil {
		return err
	}

	os.Exit(0)
	return nil
}

type node struct {
	clientset *kubernetes.Clientset
	name      string
}

func (n *node) getNodeLabelValue(label string) (string, error) {
	node, err := n.clientset.CoreV1().Nodes().Get(context.TODO(), n.name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("unable to get node object: %w", err)
	}

	value, ok := node.Labels[label]
	if !ok {
		return "", nil
	}

	return value, nil
}

func (n *node) setNodeLabelValue(label, value string) error {
	node, err := n.clientset.CoreV1().Nodes().Get(context.TODO(), n.name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get node object: %w", err)
	}

	labels := node.GetLabels()
	labels[label] = value
	node.SetLabels(labels)
	_, err = n.clientset.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("unable to update node object: %w", err)
	}

	return nil
}
