/*
 * Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
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

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
)

const (
	migPartedCliName = "nvidia-mig-parted"

	configStateRebooting = "rebooting"

	ldPreloadEnvVar = "LD_PRELOAD"
)

var (
	hostGPUClientServicesStopped []string

	systemdServicePrefixPattern = regexp.MustCompile(`^[a-zA-Z0-9:._\\-]+\.(service|socket|device|mount|automount|swap|target|path|timer|slice|scope)$`)
)

// reconfigureMIGOptions contains configuration options for reconfiguring MIG
// settings on a Kubernetes node. This struct is used to manage the various
// parameters required for applying MIG configurations through mig-parted, including node identification, configuration files, reboot behavior, and host
// system service management.
type reconfigureMIGOptions struct {
	// NodeName is the kubernetes node to change the MIG configuration on.
	// Its validation follows the RFC 1123 standard for DNS subdomain names.
	// Source: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
	NodeName string `validate:"required,hostname_rfc1123"`

	// MIGPartedConfigFile is the mig-parted configuration file path.
	MIGPartedConfigFile string `validate:"required,filepath"`

	// SelectedMIGConfig is the selected mig-parted configuration to apply to the
	// node.
	SelectedMIGConfig string

	// DriverLibrayPath is the path to libnvidia-ml.so.1 in the container.
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
	HostMIGManagerStateFile string `validate:"filepath"`

	// HostGPUClientServices is a comma separated list of host systemd services to
	// shutdown/restart across a MIG reconfiguration.
	HostGPUClientServices []string `validate:"dive,systemd_service_name"`

	// HostKubeletService is the name of the host's 'kubelet' systemd service
	// which may need to be shutdown/restarted across a MIG mode reconfiguration.
	HostKubeletService string `validate:"systemd_service_name"`
}

// reconfigureMIG configures MIG (Multi-Instance GPU) settings on a Kubernetes
// node. It validates the requested configuration, checks the current state,
// applies MIG mode changes, manages host GPU client services, and handles
// reboots when necessary. The function ensures that MIG configurations are
// applied safely with proper service lifecycle management.
func reconfigureMIG(clientset *kubernetes.Clientset, opts *reconfigureMIGOptions) error {
	log.Info("Validating reconfigure MIG options")
	if err := validate(opts); err != nil {
		log.Errorf("%v", err)
		return err
	}

	log.Info("Asserting that the requested configuration is present in the configuration file")
	if err := assertValidMIGConfig(opts); err != nil {
		log.Error("Unable to validate the selected MIG configuration")
		return err
	}

	log.Infof("Getting current value of the '%s' node label", vGPUConfigStateLabel)
	state, err := getNodeLabelValue(clientset, vGPUConfigStateLabel)
	if err != nil {
		log.Errorf("Unable to get the value of the '%s' label", vGPUConfigStateLabel)
		return err
	}
	log.Infof("Current value of '%s=%s'", vGPUConfigStateLabel, state)

	log.Info("Checking if the selected MIG config is currently applied or not")
	if err := assertMIGConfig(opts); err == nil {
		log.Info("MIG configuration already applied")
		return nil
	}

	if opts.HostRootMount != "" && opts.HostMIGManagerStateFile != "" {
		stateFilePath := filepath.Join(opts.HostRootMount, opts.HostMIGManagerStateFile)
		if _, err := os.Stat(stateFilePath); err == nil {
			log.Infof("Persisting %s to %s", opts.SelectedMIGConfig, opts.HostMIGManagerStateFile)
			if err := hostPersistConfig(opts); err != nil {
				log.Errorf("Unable to persist %s to %s", opts.SelectedMIGConfig, opts.HostMIGManagerStateFile)
				return err
			}
		}
	}

	log.Info("Checking if the MIG mode setting in the selected config is currently applied or not")
	log.Infof("If the state is '%s', we expect this to always return true", configStateRebooting)
	migModeChangeRequired := false
	if err := assertMIGModeOnly(opts); err != nil {
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
		if err := hostStopSystemdServices(opts); err != nil {
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
	if err := applyMIGModeOnly(opts); err != nil || assertMIGModeOnly(opts) != nil {
		if opts.WithReboot {
			log.Infof("Changing the '%s' node label to '%s'", vGPUConfigStateLabel, configStateRebooting)
			if err := setNodeLabelValue(clientset, vGPUConfigStateLabel, configStateRebooting); err != nil {
				log.Errorf("Unable to set the value of '%s' to '%s'", vGPUConfigStateLabel, configStateRebooting)
				log.Error("Exiting so as not to reboot multiple times unexpectedly")
				return err
			}
			return rebootHost(opts.HostRootMount)
		}
	}

	log.Info("Applying the selected MIG config to the node")
	if err := applyMIGConfig(opts); err != nil {
		return err
	}

	if opts.WithShutdownHostGPUClients {
		log.Info("Restarting all GPU clients previously shutdown on the host by restarting their systemd services")
		if err := hostStartSystemdServices(opts); err != nil {
			log.Error("Unable to restart GPU clients on host by restarting their systemd services")
			return err
		}
	}

	return nil
}

func assertValidMIGConfig(opts *reconfigureMIGOptions) error {
	args := []string{
		"--debug",
		"assert",
		"--valid-config",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	cmd := migPartedCmd(opts.DriverLibraryPath, args...)
	return runCommandWithOutput(cmd)
}

func assertMIGConfig(opts *reconfigureMIGOptions) error {
	args := []string{
		"--debug",
		"assert",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	cmd := migPartedCmd(opts.DriverLibraryPath, args...)
	return runCommandWithOutput(cmd)
}

func assertMIGModeOnly(opts *reconfigureMIGOptions) error {
	args := []string{
		"--debug",
		"assert",
		"--mode-only",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	cmd := migPartedCmd(opts.DriverLibraryPath, args...)
	return runCommandWithOutput(cmd)
}

func applyMIGModeOnly(opts *reconfigureMIGOptions) error {
	args := []string{
		"--debug",
		"apply",
		"--mode-only",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	cmd := migPartedCmd(opts.DriverLibraryPath, args...)
	return runCommandWithOutput(cmd)
}

func applyMIGConfig(opts *reconfigureMIGOptions) error {
	args := []string{
		"--debug",
		"apply",
		"--config-file", opts.MIGPartedConfigFile,
		"--selected-config", opts.SelectedMIGConfig,
	}
	cmd := migPartedCmd(opts.DriverLibraryPath, args...)
	return runCommandWithOutput(cmd)
}

func hostPersistConfig(opts *reconfigureMIGOptions) error {
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

func hostStopSystemdServices(opts *reconfigureMIGOptions) error {
	hostGPUClientServicesStopped = []string{}

	for _, service := range opts.HostGPUClientServices {
		if err := processSystemdService(opts, service, "stop"); err != nil {
			return err
		}
	}
	return nil
}

func hostStartSystemdServices(opts *reconfigureMIGOptions) error {
	if len(hostGPUClientServicesStopped) == 0 {
		for _, service := range opts.HostGPUClientServices {
			if shouldRestartService(opts, service) {
				hostGPUClientServicesStopped = append(hostGPUClientServicesStopped, service)
			}
		}
	}

	retCode := 0
	for _, service := range hostGPUClientServicesStopped {
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
			hostGPUClientServicesStopped = append([]string{service}, hostGPUClientServicesStopped...)
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
			hostGPUClientServicesStopped = append([]string{service}, hostGPUClientServicesStopped...)
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
		hostGPUClientServicesStopped = append([]string{service}, hostGPUClientServicesStopped...)
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

func migPartedCmd(driverLibraryPath string, args ...string) *exec.Cmd {
	cmd := exec.Command(migPartedCliName, args...)
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%s", ldPreloadEnvVar, driverLibraryPath))
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

func validate(opts *reconfigureMIGOptions) error {
	validate := validator.New(validator.WithRequiredStructEnabled())

	err := validate.RegisterValidation("systemd_service_name", validateSystemdServiceName)
	if err != nil {
		return fmt.Errorf("unable to register systemd service name validator: %w", err)
	}
	err = validate.Struct(opts)
	if err != nil {
		return fmt.Errorf("unable to validate the reconfigure MIG options: %w", err)
	}
	return nil
}

// validateSystemdServiceName validates a systemd service name according to systemd naming rules.
// The unit name prefix must consist of one or more valid characters (ASCII letters, digits, ":", "-", "_", ".", and "\").
// The total length of the unit name including the suffix must not exceed 255 characters.
// The unit type suffix must be one of ".service", ".socket", ".device", ".mount", ".automount", ".swap", ".target", ".path", ".timer", ".slice", or ".scope".
// Source: https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html
func validateSystemdServiceName(fl validator.FieldLevel) bool {
	serviceName := fl.Field().String()

	if len(serviceName) == 0 || len(serviceName) > 255 {
		return false
	}

	validSuffixes := []string{
		".service",
		".socket",
		".device",
		".mount",
		".automount",
		".swap",
		".target",
		".path",
		".timer",
		".slice",
		".scope",
	}

	hasSuffix := false
	for _, suffix := range validSuffixes {
		if strings.HasSuffix(serviceName, suffix) {
			hasSuffix = true
			break
		}
	}

	if !hasSuffix {
		return false
	}

	return systemdServicePrefixPattern.MatchString(serviceName)
}
