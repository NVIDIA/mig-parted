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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	migPartedCliName = "nvidia-mig-parted"

	configStateRebooting = "rebooting"

	ldPreloadEnvVar = "LD_PRELOAD"
)

// New creates a MIG Reconfigurer with the supplied options.
func New(opts ...Option) (Reconfigurer, error) {
	o := &reconfigureMIGOptions{}

	for _, opt := range opts {
		opt(o)
	}

	if err := o.Validate(); err != nil {
		return nil, err
	}

	return o, nil
}

// Reconfigure configures MIG (Multi-Instance GPU) settings on a Kubernetes
// node. It validates the requested configuration, checks the current state,
// applies MIG mode changes, manages host GPU client services, and handles
// reboots when necessary. The function ensures that MIG configurations are
// applied safely with proper service lifecycle management.
func (opts *reconfigureMIGOptions) Reconfigure() error {
	log.Info("Asserting that the requested configuration is present in the configuration file")
	if err := opts.assertValidMIGConfig(); err != nil {
		log.Error("Unable to validate the selected MIG configuration")
		return err
	}

	log.Infof("Getting current value of the '%s' node label", opts.configStateLabel)
	state, err := opts.getNodeLabelValue(opts.configStateLabel)
	if err != nil {
		log.Errorf("Unable to get the value of the '%s' label", opts.configStateLabel)
		return err
	}
	log.Infof("Current value of '%s=%s'", opts.configStateLabel, state)

	log.Info("Checking if the selected MIG config is currently applied or not")
	if err := opts.assertMIGConfig(); err == nil {
		log.Info("MIG configuration already applied")
		return nil
	}

	if opts.HostRootMount != "" && opts.HostMIGManagerStateFile != "" {
		stateFilePath := filepath.Join(opts.HostRootMount, opts.HostMIGManagerStateFile)
		if _, err := os.Stat(stateFilePath); err == nil {
			log.Infof("Persisting %s to %s", opts.SelectedMIGConfig, opts.HostMIGManagerStateFile)
			if err := opts.hostPersistConfig(); err != nil {
				log.Errorf("Unable to persist %s to %s", opts.SelectedMIGConfig, opts.HostMIGManagerStateFile)
				return err
			}
		}
	}

	log.Info("Checking if the MIG mode setting in the selected config is currently applied or not")
	log.Infof("If the state is '%s', we expect this to always return true", configStateRebooting)
	migModeChangeRequired := false
	if err := opts.assertMIGModeOnly(); err != nil {
		if state == configStateRebooting {
			log.Error("MIG mode change did not take effect after rebooting")
			return fmt.Errorf("MIG mode change failed after reboot")
		}
		if opts.WithShutdownHostGPUClients {
			opts.HostGPUClientServices = append(opts.HostGPUClientServices, opts.HostKubeletService)
		}
		migModeChangeRequired = true
	}

	if opts.WithShutdownHostGPUClients {
		log.Info("Shutting down all GPU clients on the host by stopping their systemd services")
		if err := opts.hostStopSystemdServices(); err != nil {
			log.Error("Unable to shutdown GPU clients on host by stopping their systemd services")
			return err
		}
		if migModeChangeRequired {
			log.Info("Waiting 30 seconds for services to settle")
			time.Sleep(30 * time.Second)
		}
	}

	log.Info("Applying the MIG mode change from the selected config to the node (and double checking it took effect)")
	log.Info("If the -r option was passed, the node will be automatically rebooted if this is not successful")
	if err := opts.applyMIGModeOnly(); err != nil || opts.assertMIGModeOnly() != nil {
		if opts.WithReboot {
			log.Infof("Changing the '%s' node label to '%s'", opts.configStateLabel, configStateRebooting)
			if err := opts.setNodeLabelValue(opts.configStateLabel, configStateRebooting); err != nil {
				log.Errorf("Unable to set the value of '%s' to '%s'", opts.configStateLabel, configStateRebooting)
				log.Error("Exiting so as not to reboot multiple times unexpectedly")
				return err
			}
			return rebootHost(opts.HostRootMount)
		}
	}

	log.Info("Applying the selected MIG config to the node")
	if err := opts.applyMIGConfig(); err != nil {
		return err
	}

	if opts.WithShutdownHostGPUClients {
		log.Info("Restarting all GPU clients previously shutdown on the host by restarting their systemd services")
		if err := opts.hostStartSystemdServices(); err != nil {
			log.Error("Unable to restart GPU clients on host by restarting their systemd services")
			return err
		}
	}

	return nil
}

func (opts *reconfigureMIGOptions) assertValidMIGConfig() error {
	args := []string{
		"--debug",
		"assert",
		"--valid-config",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	return opts.runMigParted(args...)
}

func (opts *reconfigureMIGOptions) assertMIGConfig() error {
	args := []string{
		"--debug",
		"assert",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	return opts.runMigParted(args...)
}

func (opts *reconfigureMIGOptions) assertMIGModeOnly() error {
	args := []string{
		"--debug",
		"assert",
		"--mode-only",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	return opts.runMigParted(args...)
}

func (opts *reconfigureMIGOptions) applyMIGModeOnly() error {
	args := []string{
		"--debug",
		"apply",
		"--mode-only",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	return opts.runMigParted(args...)
}

func (opts *reconfigureMIGOptions) applyMIGConfig() error {
	args := []string{
		"--debug",
		"apply",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	return opts.runMigParted(args...)
}

func (opts *reconfigureMIGOptions) hostPersistConfig() error {
	config := fmt.Sprintf(`[Service]
Environment="MIG_PARTED_SELECTED_CONFIG=%s"
`, opts.SelectedMIGConfig)

	stateFilePath := filepath.Join(opts.HostRootMount, opts.HostMIGManagerStateFile)
	// #nosec G306 -- We cannot use 0600 here as the file is read by systemd.
	if err := os.WriteFile(stateFilePath, []byte(config), 0644); err != nil {
		return err
	}

	cmd := exec.Command("chroot", opts.HostRootMount, "systemctl", "daemon-reload") // #nosec G204 -- HostRootMount is validated via dirpath validator.
	return runCommandWithOutput(cmd)
}

func (opts *reconfigureMIGOptions) hostStopSystemdServices() error {
	opts.hostGPUClientServicesStopped = []string{}

	for _, service := range opts.HostGPUClientServices {
		if err := processSystemdService(opts, service, "stop"); err != nil {
			return err
		}
	}
	return nil
}

func (opts *reconfigureMIGOptions) hostStartSystemdServices() error {
	if len(opts.hostGPUClientServicesStopped) == 0 {
		for _, service := range opts.HostGPUClientServices {
			if shouldRestartService(opts, service) {
				opts.hostGPUClientServicesStopped = append(opts.hostGPUClientServicesStopped, service)
			}
		}
	}

	retCode := 0
	for _, service := range opts.hostGPUClientServicesStopped {
		log.Infof("Starting %s", service)
		cmd := exec.Command("chroot", opts.HostRootMount, "systemctl", "start", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
		if err := cmd.Run(); err != nil {
			log.Errorf("Error Starting %s: skipping, but continuing...", service)
			retCode = 1
		}
	}

	if retCode != 0 {
		return fmt.Errorf("some services failed to start")
	}
	return nil
}

func processSystemdService(opts *reconfigureMIGOptions, service, action string) error {
	cmd := exec.Command("chroot", opts.HostRootMount, "systemctl", "-q", "is-active", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	if err := cmd.Run(); err == nil {
		log.Infof("%s %s (active, will-restart)", action, service)
		cmd = exec.Command("chroot", opts.HostRootMount, "systemctl", action, service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name, action is controlled parameter.
		if err := cmd.Run(); err != nil {
			return err
		}
		if action == "stop" {
			opts.hostGPUClientServicesStopped = append([]string{service}, opts.hostGPUClientServicesStopped...)
		}
		return nil
	}

	cmd = exec.Command("chroot", opts.HostRootMount, "systemctl", "-q", "is-enabled", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	if err := cmd.Run(); err != nil {
		log.Infof("Skipping %s (no-exist)", service)
		return nil
	}

	cmd = exec.Command("chroot", opts.HostRootMount, "systemctl", "-q", "is-failed", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	if err := cmd.Run(); err == nil {
		log.Infof("Skipping %s (is-failed, will-restart)", service)
		if action == "stop" {
			opts.hostGPUClientServicesStopped = append([]string{service}, opts.hostGPUClientServicesStopped...)
		}
		return nil
	}

	cmd = exec.Command("chroot", opts.HostRootMount, "systemctl", "-q", "is-enabled", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	if err := cmd.Run(); err != nil {
		log.Infof("Skipping %s (disabled)", service)
		return nil
	}

	cmd = exec.Command("chroot", opts.HostRootMount, "systemctl", "show", "--property=Type", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) == "Type=oneshot" {
		log.Infof("Skipping %s (inactive, oneshot, no-restart)", service)
		return nil
	}

	log.Infof("Skipping %s (inactive, will-restart)", service)
	if action == "stop" {
		opts.hostGPUClientServicesStopped = append([]string{service}, opts.hostGPUClientServicesStopped...)
	}
	return nil
}

func shouldRestartService(opts *reconfigureMIGOptions, service string) bool {
	cmd := exec.Command("chroot", opts.HostRootMount, "systemctl", "-q", "is-active", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	if err := cmd.Run(); err == nil {
		return false
	}

	cmd = exec.Command("chroot", opts.HostRootMount, "systemctl", "-q", "is-enabled", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	if err := cmd.Run(); err != nil {
		return false
	}

	cmd = exec.Command("chroot", opts.HostRootMount, "systemctl", "-q", "is-failed", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	if err := cmd.Run(); err == nil {
		return true
	}

	cmd = exec.Command("chroot", opts.HostRootMount, "systemctl", "-q", "is-enabled", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	if err := cmd.Run(); err != nil {
		return false
	}

	cmd = exec.Command("chroot", opts.HostRootMount, "systemctl", "show", "--property=Type", service) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	if strings.TrimSpace(string(output)) == "Type=oneshot" {
		return false
	}

	return true
}

func runCommandWithOutput(cmd *exec.Cmd) error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (opts *reconfigureMIGOptions) runMigParted(args ...string) error {
	cmd := opts.migPartedCmd(args...)
	return runCommandWithOutput(cmd)
}

func (opts *reconfigureMIGOptions) migPartedCmd(args ...string) *exec.Cmd {
	cmd := exec.Command(migPartedCliName, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", ldPreloadEnvVar, opts.DriverLibraryPath))
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

func (opts *reconfigureMIGOptions) getNodeLabelValue(label string) (string, error) {
	node, err := opts.clientset.CoreV1().Nodes().Get(context.TODO(), opts.NodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("unable to get node object: %w", err)
	}

	value, ok := node.Labels[label]
	if !ok {
		return "", nil
	}

	return value, nil
}

func (opts *reconfigureMIGOptions) setNodeLabelValue(label, value string) error {
	node, err := opts.clientset.CoreV1().Nodes().Get(context.TODO(), opts.NodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get node object: %w", err)
	}

	labels := node.GetLabels()
	labels[label] = value
	node.SetLabels(labels)
	_, err = opts.clientset.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("unable to update node object: %w", err)
	}

	return nil
}
