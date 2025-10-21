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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"tags.cncf.io/container-device-interface/pkg/cdi"

	"github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi"
	transformroot "github.com/NVIDIA/nvidia-container-toolkit/pkg/nvcdi/transform/root"

	"github.com/NVIDIA/mig-parted/internal/systemd"
)

const (
	migConfigStateLabel            = "nvidia.com/mig.config.state"
	devicePluginDeployLabel        = "nvidia.com/gpu.deploy.device-plugin"
	gpuFeatureDiscoveryDeployLabel = "nvidia.com/gpu.deploy.gpu-feature-discovery"
	dcgmExporterDeployLabel        = "nvidia.com/gpu.deploy.dcgm-exporter"
	dcgmDeployLabel                = "nvidia.com/gpu.deploy.dcgm"
	nvsmDeployLabel                = "nvidia.com/gpu.deploy.nvsm"

	migStateSuccess = "success"
	migStateFailed  = "failed"
)

var (
	NoRestartHostSystemdServicesOnExit = false
	NoRestartK8sDaemonsetsOnExit       = false
)

// Options holds all the command line opts for the reconfigure command
type Options struct {
	// Required opts
	NodeName                   string
	MigConfigFile              string
	SelectedMigConfig          string
	DefaultGPUClientsNamespace string

	// Optional opts
	WithReboot                 bool
	WithShutdownHostGPUClients bool
	CDIEnabled                 bool
	HostRootMount              string
	HostNvidiaDir              string
	HostMigManagerStateFile    string
	HostGPUClientServices      string
	HostKubeletService         string
	DriverRoot                 string
	DriverRootCtrPath          string
	DevRoot                    string
	DevRootCtrPath             string
	DriverLibraryPath          string
	NvidiaSMIPath              string
	NvidiaCDIHookPath          string
}

// Reconfigure handles the MIG reconfiguration process
type Reconfigure struct {
	ctx            context.Context
	clientset      *kubernetes.Clientset
	systemdManager *systemd.Manager
	opts           *Options

	migPartedBinary []string

	// State tracking
	pluginDeployed        string
	gfdDeployed           string
	dcgmExporterDeployed  string
	dcgmDeployed          string
	nvsmDeployed          string
	currentState          string
	migModeChangeRequired bool
	stoppedServices       []string
}

// New creates a new Reconfigure instance
func New(ctx context.Context, clientset *kubernetes.Clientset, migPartedBinary []string, opts *Options) (*Reconfigure, error) {
	if len(opts.HostRootMount) > 0 {
		hostSystemBusAddress := fmt.Sprintf("unix:path=%s/run/dbus/system_bus_socket", opts.HostRootMount)
		_ = os.Setenv("DBUS_SYSTEM_BUS_ADDRESS", hostSystemBusAddress)
	}

	systemdManager, err := systemd.NewManager(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize systemd manager: %w", err)
	}

	return &Reconfigure{
		ctx:             ctx,
		clientset:       clientset,
		migPartedBinary: migPartedBinary,
		opts:            opts,
		systemdManager:  systemdManager,
	}, nil
}

// Run executes the complete MIG reconfiguration process
func (r *Reconfigure) Run() error {

	// Ensure systemd managers are cleaned up
	defer r.cleanup()

	if err := r.getCurrentNodeLabels(); err != nil {
		_ = r.setState(migStateFailed)
		return fmt.Errorf("failed to get current node labels: %w", err)
	}

	if err := r.validateMigConfig(); err != nil {
		_ = r.setState(migStateFailed)
		return fmt.Errorf("failed to validate MIG configuration: %w", err)
	}

	if r.isConfigAlreadyApplied() {
		log.Info("Selected MIG config is already applied")
		_ = r.setState(migStateSuccess)
		return nil
	}

	if err := r.persistConfigIfNeeded(); err != nil {
		_ = r.setState(migStateFailed)
		return fmt.Errorf("failed to persist config: %w", err)
	}

	if err := r.checkMigModeChangeRequired(); err != nil {
		_ = r.setState(migStateFailed)
		return fmt.Errorf("failed to check if a MIG mode change is required: %w", err)
	}

	if err := r.setNodeLabel(migConfigStateLabel, "pending"); err != nil {
		log.Info("Unable to set the value of 'nvidia.com/mig.config.state' to 'pending'")
		_ = r.setState(migStateFailed)
		return fmt.Errorf("failed to set state to pending: %w", err)
	}

	if err := r.shutdownKubernetesGPUClients(); err != nil {
		log.Info("Unable to tear down GPU client pods by setting their daemonset labels")
		_ = r.setState(migStateFailed)
		return fmt.Errorf("failed to shutdown Kubernetes GPU clients: %w", err)
	}

	if err := r.waitForPodsToBeDeleted(); err != nil {
		return fmt.Errorf("failed to wait for pods to be deleted: %w", err)
	}

	if r.opts.WithShutdownHostGPUClients {
		if err := r.shutdownHostGPUClients(); err != nil {
			_ = r.setState(migStateFailed)
			return fmt.Errorf("failed to shutdown host GPU clients: %w", err)
		}
		if r.migModeChangeRequired {
			time.Sleep(30 * time.Second)
		}
	}

	if err := r.applyMigModeChange(); err != nil {
		if r.opts.WithReboot {
			log.Infof("Changing the '%s' node label to 'rebooting'\n", migConfigStateLabel)

			if err := r.setNodeLabel(migConfigStateLabel, "rebooting"); err != nil {
				log.Info("Unable to set the value of 'nvidia.com/mig.config.state' to 'rebooting'")
				log.Info("Exiting so as not to reboot multiple times unexpectedly")
				_ = r.setState(migStateFailed)
				return err
			}

			log.Info("Rebooting the node...")
			cmd := exec.Command("chroot", r.opts.HostRootMount, "reboot")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Warnf("Failed to reboot the node: %v", err)
			}
			return nil
		}
	}

	if err := r.applyMigConfig(); err != nil {
		_ = r.setState(migStateFailed)
		return fmt.Errorf("failed to apply MIG configuration: %w", err)
	}

	if r.opts.CDIEnabled {
		log.Info("Running nvidia-smi")
		if err := r.runNvidiaSMI(); err != nil {
			_ = r.setState(migStateFailed)
			return fmt.Errorf("failed to run nvidia-smi: %w", err)
		}
		if err := r.handleCDI(); err != nil {
			_ = r.setState(migStateFailed)
			return fmt.Errorf("failed to handle CDI: %w", err)
		}
	}

	if r.opts.WithShutdownHostGPUClients {
		if err := r.restartHostGPUClients(); err != nil {
			_ = r.setState(migStateFailed)
			return fmt.Errorf("failed to restart host GPU clients: %w", err)
		}
	}

	if err := r.restartValidatorPod(); err != nil {
		return fmt.Errorf("failed to restart validator pod: %w", err)
	}

	if err := r.restartKubernetesGPUClients(); err != nil {
		_ = r.setState(migStateFailed)
		return fmt.Errorf("failed to restart Kubernetes GPU clients: %w", err)
	}

	return r.setState(migStateSuccess)
}

// cleanup cleans up systemd managers
func (r *Reconfigure) cleanup() {
	if r.systemdManager != nil {
		r.systemdManager.Close()
	}
}

// getCurrentNodeLabels retrieves current node labels
func (r *Reconfigure) getCurrentNodeLabels() error {
	node, err := r.clientset.CoreV1().Nodes().Get(r.ctx, r.opts.NodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	labels := node.Labels
	r.pluginDeployed = labels[devicePluginDeployLabel]
	r.gfdDeployed = labels[gpuFeatureDiscoveryDeployLabel]
	r.dcgmExporterDeployed = labels[dcgmExporterDeployLabel]
	r.dcgmDeployed = labels[dcgmDeployLabel]
	r.nvsmDeployed = labels[nvsmDeployLabel]
	r.currentState = labels[migConfigStateLabel]

	log.Infof("Current value of '%s=%s'\n", devicePluginDeployLabel, r.pluginDeployed)
	log.Infof("Current value of '%s=%s'\n", gpuFeatureDiscoveryDeployLabel, r.gfdDeployed)
	log.Infof("Current value of '%s=%s'\n", dcgmExporterDeployLabel, r.dcgmExporterDeployed)
	log.Infof("Current value of '%s=%s'\n", dcgmDeployLabel, r.dcgmDeployed)
	log.Infof("Current value of '%s=%s'\n", nvsmDeployLabel, r.nvsmDeployed)
	log.Infof("Current value of '%s=%s'\n", migConfigStateLabel, r.currentState)

	return nil
}

// validateMigConfig validates the MIG configuration
func (r *Reconfigure) validateMigConfig() error {
	log.Info("Asserting that the requested configuration is present in the configuration file")

	commandSlice := r.migPartedBinary
	commandArgs := []string{"assert", "--valid-config", "-f", r.opts.MigConfigFile, "-c", r.opts.SelectedMigConfig}
	commandSlice = append(commandSlice, commandArgs...)

	// TODO: need to invoke this method via Go
	cmd := exec.Command(commandSlice[0], commandSlice[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// isConfigAlreadyApplied checks if the selected config is already applied
func (r *Reconfigure) isConfigAlreadyApplied() bool {
	log.Info("Checking if the selected MIG config is currently applied or not")

	commandSlice := r.migPartedBinary
	commandArgs := []string{"assert", "-f", r.opts.MigConfigFile, "-c", r.opts.SelectedMigConfig}
	commandSlice = append(commandSlice, commandArgs...)

	// TODO: need to invoke this method via Go
	cmd := exec.Command(commandSlice[0], commandSlice[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run() == nil
}

// persistConfigIfNeeded persists the configuration if needed
func (r *Reconfigure) persistConfigIfNeeded() error {
	if r.opts.HostRootMount == "" || r.opts.HostMigManagerStateFile == "" {
		return nil
	}

	stateFilePath := filepath.Join(r.opts.HostRootMount, r.opts.HostMigManagerStateFile)
	if _, err := os.Stat(stateFilePath); os.IsNotExist(err) {
		return nil
	}

	log.Infof("Persisting %s to %s\n", r.opts.SelectedMigConfig, r.opts.HostMigManagerStateFile)
	return r.persistConfig()
}

// persistConfig persists the configuration to the state file
func (r *Reconfigure) persistConfig() error {
	config := fmt.Sprintf(`[Service]
Environment="MIG_PARTED_SELECTED_CONFIG=%s"
`, r.opts.SelectedMigConfig)

	stateFilePath := filepath.Join(r.opts.HostRootMount, r.opts.HostMigManagerStateFile)

	// Write config to file
	if err := os.WriteFile(stateFilePath, []byte(config), 0600); err != nil {
		return fmt.Errorf("failed to write config to state file: %w", err)
	}

	return r.systemdManager.ReloadDaemon()
}

// checkMigModeChangeRequired checks if MIG mode change is required
func (r *Reconfigure) checkMigModeChangeRequired() error {
	log.Info("Checking if the MIG mode setting in the selected config is currently applied or not")
	log.Info("If the state is 'rebooting', we expect this to always return true")

	commandSlice := r.migPartedBinary
	commandArgs := []string{"assert", "--mode-only", "-f", r.opts.MigConfigFile, "-c", r.opts.SelectedMigConfig}
	commandSlice = append(commandSlice, commandArgs...)

	// TODO: need to invoke this method via Go
	cmd := exec.Command(commandSlice[0], commandSlice[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if r.currentState == "rebooting" {
			return fmt.Errorf("MIG mode change did not take effect after rebooting")
		}
		if r.opts.WithShutdownHostGPUClients {
			// Add kubelet service to the list if we're shutting down host GPU clients
			if r.opts.HostKubeletService != "" {
				services := strings.Split(r.opts.HostGPUClientServices, ",")
				services = append(services, r.opts.HostKubeletService)
				r.opts.HostGPUClientServices = strings.Join(services, ",")
			}
		}
		r.migModeChangeRequired = true
	}

	return nil
}

// shutdownKubernetesGPUClients shuts down GPU clients in Kubernetes
func (r *Reconfigure) shutdownKubernetesGPUClients() error {
	log.Info("Shutting down all GPU clients in Kubernetes by disabling their component-specific nodeSelector labels")

	labels := map[string]string{
		devicePluginDeployLabel:        r.maybeSetPaused(r.pluginDeployed),
		gpuFeatureDiscoveryDeployLabel: r.maybeSetPaused(r.gfdDeployed),
		dcgmExporterDeployLabel:        r.maybeSetPaused(r.dcgmExporterDeployed),
		dcgmDeployLabel:                r.maybeSetPaused(r.dcgmDeployed),
		nvsmDeployLabel:                r.maybeSetPaused(r.nvsmDeployed),
	}

	return r.setNodeLabels(labels)
}

// waitForPodsToBeDeleted waits for GPU client pods to be deleted
func (r *Reconfigure) waitForPodsToBeDeleted() error {
	timeout := 5 * time.Minute

	log.Infof("Waiting for the device-plugin to shutdown")
	if err := r.waitForPodDeletion("app=nvidia-device-plugin-daemonset", timeout); err != nil {
		return fmt.Errorf("device-plugin pod did not shutdown: %w", err)
	}

	log.Info("Waiting for gpu-feature-discovery to shutdown")
	if err := r.waitForPodDeletion("app=gpu-feature-discovery", timeout); err != nil {
		return fmt.Errorf("gpu-feature-discovery pod did not shutdown: %w", err)
	}

	log.Info("Waiting for dcgm-exporter to shutdown")
	if err := r.waitForPodDeletion("app=nvidia-dcgm-exporter", timeout); err != nil {
		return fmt.Errorf("dcgm-exporter pod did not shutdown: %w", err)
	}

	log.Info("Waiting for dcgm to shutdown")
	if err := r.waitForPodDeletion("app=nvidia-dcgm", timeout); err != nil {
		return fmt.Errorf("dcgm pod did not shutdown: %w", err)
	}

	log.Info("Removing the cuda-validator pod")
	if err := r.deletePod("app=nvidia-cuda-validator"); err != nil {
		return fmt.Errorf("failed to delete cuda-validator pod: %w", err)
	}

	log.Info("Removing the plugin-validator pod")
	if err := r.deletePod("app=nvidia-device-plugin-validator"); err != nil {
		return fmt.Errorf("failed to delete plugin-validator pod: %w", err)
	}

	return nil
}

// shutdownHostGPUClients shuts down host GPU clients
func (r *Reconfigure) shutdownHostGPUClients() error {
	log.Info("Shutting down all GPU clients on the host by stopping their systemd services")

	services := strings.Split(r.opts.HostGPUClientServices, ",")
	stoppedServices, err := r.systemdManager.StopSystemdServices(services)
	if err != nil {
		return fmt.Errorf("failed to stop host systemd services: %w", err)
	}
	r.stoppedServices = stoppedServices
	return nil
}

// applyMigModeChange applies the MIG mode change
func (r *Reconfigure) applyMigModeChange() error {
	log.Info("Applying the MIG mode change from the selected config to the node (and double checking it took effect)")
	log.Info("If the -r option was passed, the node will be automatically rebooted if this is not successful")

	commandSlice := r.migPartedBinary
	commandArgs := []string{"-d", "apply", "--mode-only", "-f", r.opts.MigConfigFile, "-c", r.opts.SelectedMigConfig}
	commandSlice = append(commandSlice, commandArgs...)

	// TODO: need to invoke this method via Go
	cmd := exec.Command(commandSlice[0], commandSlice[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply MIG mode change: %w", err)
	}

	commandSlice = r.migPartedBinary
	commandArgs = []string{"-d", "assert", "--mode-only", "-f", r.opts.MigConfigFile, "-c", r.opts.SelectedMigConfig}
	commandSlice = append(commandSlice, commandArgs...)

	cmd = exec.Command(commandSlice[0], commandSlice[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// applyMigConfig applies the MIG configuration
func (r *Reconfigure) applyMigConfig() error {
	log.Info("Applying the selected MIG config to the node")

	commandSlice := r.migPartedBinary
	commandArgs := []string{"-d", "apply", "-f", r.opts.MigConfigFile, "-c", r.opts.SelectedMigConfig}
	commandSlice = append(commandSlice, commandArgs...)

	cmd := exec.Command(commandSlice[0], commandSlice[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// handleCDI handles CDI operations if enabled
func (r *Reconfigure) handleCDI() error {

	log.Info("Creating NVIDIA control device nodes")
	// TODO: Instead of shelling out, we need to invoke the method via Go. The Toolkit code needs to be refactored first.
	cmd := exec.Command("nvidia-ctk", "system", "create-device-nodes", "--control-devices", "--dev-root="+r.opts.DevRootCtrPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create control device nodes: %w", err)
	}

	log.Info("Creating management CDI spec")
	return r.createCDISpec()
}

// restartHostGPUClients restarts host GPU clients
func (r *Reconfigure) restartHostGPUClients() error {
	log.Info("Restarting all GPU clients previously shutdown on the host by restarting their systemd services")

	NoRestartHostSystemdServicesOnExit = true

	if err := r.hostStartSystemdServices(); err != nil {
		log.Info("Unable to restart GPU clients on host by restarting their systemd services")
		return err
	}
	return nil
}

// restartKubernetesGPUClients restarts Kubernetes GPU clients
func (r *Reconfigure) restartKubernetesGPUClients() error {
	log.Info("Restarting all GPU clients previously shutdown in Kubernetes by reenabling their component-specific nodeSelector labels")

	NoRestartK8sDaemonsetsOnExit = true

	labels := map[string]string{
		devicePluginDeployLabel:        r.maybeSetTrue(r.pluginDeployed),
		gpuFeatureDiscoveryDeployLabel: r.maybeSetTrue(r.gfdDeployed),
		dcgmExporterDeployLabel:        r.maybeSetTrue(r.dcgmExporterDeployed),
		dcgmDeployLabel:                r.maybeSetTrue(r.dcgmDeployed),
		nvsmDeployLabel:                r.maybeSetTrue(r.nvsmDeployed),
	}

	return r.setNodeLabels(labels)
}

// restartValidatorPod restarts the validator pod
func (r *Reconfigure) restartValidatorPod() error {
	log.Info("Restarting validator pod to re-run all validations")
	return r.deletePod("app=nvidia-operator-validator")
}

// setState sets the final state and exits
func (r *Reconfigure) setState(state string) error {

	if r.opts.WithShutdownHostGPUClients && !NoRestartHostSystemdServicesOnExit {
		log.Info("Restarting any GPU clients previously shutdown on the host by restarting their systemd services")
		if err := r.hostStartSystemdServices(); err != nil {
			log.Info("Unable to restart host systemd services")
			return err
		}
	}

	if !NoRestartK8sDaemonsetsOnExit {
		log.Info("Restarting any GPU clients previously shutdown in Kubernetes by reenabling their component-specific nodeSelector labels")

		labels := map[string]string{
			devicePluginDeployLabel:        r.maybeSetTrue(r.pluginDeployed),
			gpuFeatureDiscoveryDeployLabel: r.maybeSetTrue(r.gfdDeployed),
			dcgmExporterDeployLabel:        r.maybeSetTrue(r.dcgmExporterDeployed),
			dcgmDeployLabel:                r.maybeSetTrue(r.dcgmDeployed),
			nvsmDeployLabel:                r.maybeSetTrue(r.nvsmDeployed),
		}

		if err := r.setNodeLabels(labels); err != nil {
			log.Info("Unable to bring up GPU client pods by setting their daemonset labels")
			return err
		}

	}

	log.Infof("Changing the '%s' node label to '%s'\n", migConfigStateLabel, state)
	if err := r.setNodeLabel(migConfigStateLabel, state); err != nil {
		log.Infof("Unable to set '%s' to '%s'\n", migConfigStateLabel, state)
		log.Infof("Exiting with incorrect value in '%s'\n", migConfigStateLabel)
		return err
	}
	return nil
}

func (r *Reconfigure) maybeSetPaused(currentValue string) string {
	if currentValue == "false" {
		return "false"
	}
	return "paused-for-mig-change"
}

func (r *Reconfigure) maybeSetTrue(currentValue string) string {
	if currentValue == "false" {
		return "false"
	}
	return "true"
}

func (r *Reconfigure) setNodeLabel(key, value string) error {
	return r.setNodeLabels(map[string]string{key: value})
}

func (r *Reconfigure) setNodeLabels(labels map[string]string) error {
	node, err := r.clientset.CoreV1().Nodes().Get(r.ctx, r.opts.NodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	for key, value := range labels {
		node.Labels[key] = value
	}

	_, err = r.clientset.CoreV1().Nodes().Update(r.ctx, node, metav1.UpdateOptions{})
	return err
}

func (r *Reconfigure) waitForPodDeletion(labelSelector string, timeout time.Duration) error {
	return wait.PollUntilContextTimeout(r.ctx, time.Second, timeout, false, func(ctx context.Context) (bool, error) {
		pods, err := r.clientset.CoreV1().Pods(r.opts.DefaultGPUClientsNamespace).List(ctx, metav1.ListOptions{
			FieldSelector: "spec.nodeName=" + r.opts.NodeName,
			LabelSelector: labelSelector,
		})
		if err != nil {
			return false, err
		}
		return len(pods.Items) == 0, nil
	})
}

func (r *Reconfigure) deletePod(labelSelector string) error {
	podClient := r.clientset.CoreV1().Pods(r.opts.DefaultGPUClientsNamespace)

	podList, err := podClient.List(r.ctx, metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + r.opts.NodeName,
		LabelSelector: labelSelector,
	})
	if err != nil {
		return fmt.Errorf("failed to list pods with label %s: %w", labelSelector, err)
	}

	for _, pod := range podList.Items {
		if err := podClient.Delete(r.ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}

	return nil
}

func (r *Reconfigure) runNvidiaSMI() error {
	if r.opts.DriverRootCtrPath == r.opts.DevRootCtrPath {
		cmd := exec.Command("chroot", r.opts.DriverRootCtrPath, "nvidia-smi")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	cmd := exec.Command("chroot", r.opts.HostRootMount, r.opts.NvidiaSMIPath)
	cmd.Env = append(os.Environ(), "LD_PRELOAD="+r.opts.DriverLibraryPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *Reconfigure) createCDISpec() error {
	log.Info("Creating management CDI spec (simplified implementation)")

	if !r.opts.CDIEnabled {
		return nil
	}

	cdilib, err := nvcdi.New(
		// TODO: We may want to switch to klog for logging here.
		nvcdi.WithLogger(log.StandardLogger()),
		nvcdi.WithMode(nvcdi.ModeManagement),
		nvcdi.WithDriverRoot(r.opts.DriverRootCtrPath),
		nvcdi.WithDevRoot(r.opts.DevRootCtrPath),
		nvcdi.WithNVIDIACDIHookPath(r.opts.NvidiaCDIHookPath),
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
		transformroot.WithDriverRoot(r.opts.DriverRootCtrPath),
		transformroot.WithTargetDriverRoot(r.opts.DriverRoot),
		transformroot.WithDevRoot(r.opts.DevRootCtrPath),
		transformroot.WithTargetDevRoot(r.opts.DevRoot),
	)
	if err := transformer.Transform(spec.Raw()); err != nil {
		return fmt.Errorf("failed to transform driver root in CDI spec: %v", err)
	}

	name, err := cdi.GenerateNameForSpec(spec.Raw())
	if err != nil {
		return fmt.Errorf("failed to generate CDI name for management containers: %v", err)
	}
	// TODO: Should this path be configurable? What's important is that this
	// file path is the same as the one generated in the NVIDIA Container Toolkit.
	err = spec.Save(filepath.Join("/var/run/cdi/", name))
	if err != nil {
		return fmt.Errorf("failed to save CDI spec for management containers: %v", err)
	}

	return nil
}

func (r *Reconfigure) hostStartSystemdServices() error {
	services := r.stoppedServices
	var restartServices []string
	if len(services) == 0 {
		// If no services were tracked as stopped, use all configured services
		services = strings.Split(r.opts.HostGPUClientServices, ",")

		for _, service := range services {
			serviceStatus, err := r.systemdManager.GetServiceStatus(service)
			if err != nil {
				log.Infof("failed to get service status: %v\n", err)
				continue
			}

			if serviceStatus.SubState == "not-found" {
				continue
			}

			if serviceStatus.Failed {
				restartServices = append(restartServices, service)
				continue
			}

			if serviceStatus.Active || serviceStatus.Enabled || serviceStatus.Type == "oneshot" {
				continue
			}

			restartServices = append(restartServices, service)
		}
	} else {
		// Use the services that were actually stopped
		restartServices = r.stoppedServices
	}

	return r.systemdManager.StartSystemdServices(restartServices)
}
