/*
 * Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
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
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/yaml"

	"github.com/NVIDIA/mig-parted/internal/info"
	"github.com/NVIDIA/mig-parted/pkg/mig/builder"
	"github.com/NVIDIA/mig-parted/pkg/mig/discovery"
	"github.com/NVIDIA/mig-parted/pkg/mig/reconfigure"
)

const (
	ResourceNodes  = "nodes"
	MigConfigLabel = "nvidia.com/mig.config"

	DefaultReconfigureScript         = "/usr/bin/reconfigure-mig.sh"
	DefaultHostRootMount             = "/host"
	DefaultHostNvidiaDir             = "/usr/local/nvidia"
	DefaultHostMigManagerStateFile   = "/etc/systemd/system/nvidia-mig-manager.service.d/override.conf"
	DefaultHostKubeletSystemdService = "kubelet.service"
	DefaultGPUClientsNamespace       = "default"
	DefaultNvidiaDriverRoot          = "/run/nvidia/driver"
	DefaultDriverRootCtrPath         = "/run/nvidia/driver"
	DefaultNvidiaCDIHookPath         = "/usr/local/nvidia/toolkit/nvidia-cdi-hook"
	DefaultGeneratedConfigFile       = "/etc/nvidia-mig-manager/generated-config.yaml"
)

var (
	kubeconfigFlag                 string
	nodeNameFlag                   string
	configFileFlag                 string
	reconfigureScriptFlag          string
	withRebootFlag                 bool
	withShutdownHostGPUClientsFlag bool
	gpuClientsFileFlag             string
	hostRootMountFlag              string
	hostNvidiaDirFlag              string
	hostMigManagerStateFileFlag    string
	hostKubeletSystemdServiceFlag  string
	defaultGPUClientsNamespaceFlag string

	cdiEnabledFlag    bool
	driverRoot        string
	driverRootCtrPath string
	devRoot           string
	devRootCtrPath    string
	nvidiaCDIHookPath string
)

type GPUClients struct {
	Version         string   `json:"version"          yaml:"version"`
	SystemdServices []string `json:"systemd-services" yaml:"systemd-services"`
}

type SyncableMigConfig struct {
	cond     *sync.Cond
	mutex    sync.Mutex
	current  string
	lastRead string
}

func NewSyncableMigConfig() *SyncableMigConfig {
	var m SyncableMigConfig
	m.cond = sync.NewCond(&m.mutex)
	return &m
}

func (m *SyncableMigConfig) Set(value string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.current = value
	if m.current != "" {
		m.cond.Broadcast()
	}
}

func (m *SyncableMigConfig) Get() string {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.lastRead == m.current {
		m.cond.Wait()
	}
	m.lastRead = m.current
	return m.lastRead
}

func main() {
	c := cli.Command{}
	c.Before = validateFlags
	c.Action = start
	c.Version = info.GetVersionString()

	c.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "kubeconfig",
			Value:       "",
			Usage:       "absolute path to the kubeconfig file",
			Destination: &kubeconfigFlag,
			Sources:     cli.EnvVars("KUBECONFIG"),
		},
		&cli.StringFlag{
			Name:        "node-name",
			Aliases:     []string{"n"},
			Value:       "",
			Usage:       "the name of the node to watch for label changes on",
			Destination: &nodeNameFlag,
			Sources:     cli.EnvVars("NODE_NAME"),
		},
		&cli.StringFlag{
			Name:        "config-file",
			Aliases:     []string{"f"},
			Value:       "",
			Usage:       "the path to the MIG parted configuration file",
			Destination: &configFileFlag,
			Sources:     cli.EnvVars("CONFIG_FILE"),
		},
		&cli.StringFlag{
			Name:        "reconfigure-script",
			Aliases:     []string{"s"},
			Value:       DefaultReconfigureScript,
			Usage:       "script to run to do the actual MIG reconfiguration",
			Destination: &reconfigureScriptFlag,
			Sources:     cli.EnvVars("RECONFIGURE_SCRIPT"),
		},
		&cli.StringFlag{
			Name:        "host-root-mount",
			Aliases:     []string{"m"},
			Value:       DefaultHostRootMount,
			Usage:       "container path where host root directory is mounted",
			Destination: &hostRootMountFlag,
			Sources:     cli.EnvVars("HOST_ROOT_MOUNT"),
		},
		&cli.StringFlag{
			Name:        "host-nvidia-dir",
			Aliases:     []string{"i"},
			Value:       DefaultHostNvidiaDir,
			Usage:       "host path of the directory where NVIDIA managed software directory is typically located",
			Destination: &hostNvidiaDirFlag,
			Sources:     cli.EnvVars("HOST_NVIDIA_DIR"),
		},
		&cli.StringFlag{
			Name:        "host-mig-manager-state-file",
			Aliases:     []string{"o"},
			Value:       DefaultHostMigManagerStateFile,
			Usage:       "host path where the host's systemd mig-manager state file is located",
			Destination: &hostMigManagerStateFileFlag,
			Sources:     cli.EnvVars("HOST_MIG_MANAGER_STATE_FILE"),
		},
		&cli.StringFlag{
			Name:        "host-kubelet-systemd-service",
			Aliases:     []string{"k"},
			Value:       DefaultHostKubeletSystemdService,
			Usage:       "name of the host's 'kubelet' systemd service which may need to be shutdown/restarted across a MIG mode reconfiguration",
			Destination: &hostKubeletSystemdServiceFlag,
			Sources:     cli.EnvVars("HOST_KUBELET_SYSTEMD_SERVICE"),
		},
		&cli.StringFlag{
			Name:        "gpu-clients-file",
			Aliases:     []string{"g"},
			Value:       "",
			Usage:       "the path to the file listing the GPU clients that need to be shutdown across a MIG configuration",
			Destination: &gpuClientsFileFlag,
			Sources:     cli.EnvVars("GPU_CLIENTS_FILE"),
		},
		&cli.BoolFlag{
			Name:        "with-reboot",
			Aliases:     []string{"r"},
			Value:       false,
			Usage:       "reboot the node if changing the MIG mode fails for any reason",
			Destination: &withRebootFlag,
			Sources:     cli.EnvVars("WITH_REBOOT"),
		},
		&cli.BoolFlag{
			Name:        "with-shutdown-host-gpu-clients",
			Aliases:     []string{"d"},
			Value:       false,
			Usage:       "shutdown/restart any required host GPU clients across a MIG configuration",
			Destination: &withShutdownHostGPUClientsFlag,
			Sources:     cli.EnvVars("WITH_SHUTDOWN_HOST_GPU_CLIENTS"),
		},
		&cli.StringFlag{
			Name:        "default-gpu-clients-namespace",
			Aliases:     []string{"p"},
			Value:       DefaultGPUClientsNamespace,
			Usage:       "Default name of the Kubernetes namespace in which the GPU client Pods are installed in",
			Destination: &defaultGPUClientsNamespaceFlag,
			Sources:     cli.EnvVars("DEFAULT_GPU_CLIENTS_NAMESPACE"),
		},
		&cli.StringFlag{
			Name:        "nvidia-driver-root",
			Aliases:     []string{"driver-root", "t"},
			Value:       DefaultNvidiaDriverRoot,
			Usage:       "Root path to the NVIDIA driver installation. Only used if --cdi-enabled is set.",
			Destination: &driverRoot,
			Sources:     cli.EnvVars("NVIDIA_DRIVER_ROOT", "DRIVER_ROOT"),
		},
		&cli.StringFlag{
			Name:        "driver-root-ctr-path",
			Aliases:     []string{"a"},
			Value:       DefaultDriverRootCtrPath,
			Usage:       "Root path to the NVIDIA driver installation mounted in the container. Only used if --cdi-enabled is set.",
			Destination: &driverRootCtrPath,
			Sources:     cli.EnvVars("DRIVER_ROOT_CTR_PATH"),
		},
		&cli.BoolFlag{
			Name:        "cdi-enabled",
			Usage:       "Enable CDI support",
			Destination: &cdiEnabledFlag,
			Sources:     cli.EnvVars("CDI_ENABLED"),
		},
		&cli.StringFlag{
			Name:        "dev-root",
			Aliases:     []string{"b"},
			Value:       "",
			Usage:       "Root path to the NVIDIA device nodes. Only used if --cdi-enabled is set.",
			Destination: &devRoot,
			Sources:     cli.EnvVars("NVIDIA_DEV_ROOT"),
		},
		&cli.StringFlag{
			Name:        "dev-root-ctr-path",
			Aliases:     []string{"j"},
			Value:       "",
			Usage:       "Root path to the NVIDIA device nodes mounted in the container. Only used if --cdi-enabled is set.",
			Destination: &devRootCtrPath,
			Sources:     cli.EnvVars("DEV_ROOT_CTR_PATH"),
		},
		&cli.StringFlag{
			Name:        "nvidia-cdi-hook-path",
			Value:       DefaultNvidiaCDIHookPath,
			Usage:       "Path to nvidia-cdi-hook binary on the host.",
			Destination: &nvidiaCDIHookPath,
			Sources:     cli.EnvVars("NVIDIA_CDI_HOOK_PATH"),
		},
	}

	err := c.Run(context.Background(), os.Args)
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func validateFlags(ctx context.Context, c *cli.Command) (context.Context, error) {
	if nodeNameFlag == "" {
		return ctx, fmt.Errorf("invalid -n <node-name> flag: must not be empty string")
	}
	return ctx, nil
}

func createNodeConfigMap(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, configYAML []byte) error {
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		return fmt.Errorf("POD_NAMESPACE environment variable not set")
	}

	podName := os.Getenv("POD_NAME")
	if podName == "" {
		return fmt.Errorf("POD_NAME environment variable not set")
	}

	configMapName := fmt.Sprintf("%s-mig-config", nodeName)

	pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get current pod: %w", err)
	}

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component": "nvidia-mig-manager",
				"nvidia.com/node-name":        nodeName,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       pod.Name,
					UID:        pod.UID,
				},
			},
		},
		Data: map[string]string{
			"config.yaml": string(configYAML),
		},
	}

	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		existing, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get existing ConfigMap: %w", err)
		}
		configMap.ResourceVersion = existing.ResourceVersion
		_, err = clientset.CoreV1().ConfigMaps(namespace).Update(ctx, configMap, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update ConfigMap: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	log.Infof("Created/updated ConfigMap: %s/%s", namespace, configMapName)
	return nil
}

func writeGeneratedConfig() ([]byte, error) {
	configYAML, err := builder.GenerateConfigYAML()
	if err != nil {
		return nil, err
	}

	header := []byte("# DO NOT EDIT: Auto-generated MIG configuration from GPU hardware.\n" +
		"# Generated by \"nvidia-mig-manager\".\n")
	configYAML = append(header, configYAML...)

	if err := os.MkdirAll(filepath.Dir(DefaultGeneratedConfigFile), 0755); err != nil {
		return nil, err
	}

	if err := os.WriteFile(DefaultGeneratedConfigFile, configYAML, 0600); err != nil {
		return nil, err
	}

	return configYAML, nil
}

// setupMigConfig uses custom config if provided, otherwise generates MIG config from hardware.
// If dynamic generation fails with ErrNoProfilesDiscovered, falls back to DEFAULT_CONFIG_FILE.
func setupMigConfig(ctx context.Context, clientset *kubernetes.Clientset) (string, error) {
	// Use custom config if provided
	if configFileFlag != "" {
		if _, err := os.Stat(configFileFlag); err != nil {
			return "", fmt.Errorf("failed to read custom config %s: %w", configFileFlag, err)
		}
		log.Infof("Using custom config: %s", configFileFlag)
		return configFileFlag, nil
	}

	// Generate config from hardware
	log.Info("Generating MIG configuration from hardware...")
	configYAML, err := writeGeneratedConfig()
	if err != nil {
		if errors.Is(err, discovery.ErrNoProfilesDiscovered) {
			log.Warn("No MIG profiles discovered. Checking for default config")
			defaultPath := os.Getenv("DEFAULT_CONFIG_FILE")
			if defaultPath != "" {
				var statErr error
				if _, statErr = os.Stat(defaultPath); statErr == nil { //nolint:gosec // path from trusted env var set by operator
					log.Infof("Using default config: %s", defaultPath)
					return defaultPath, nil
				}
				log.Warnf("Default config file %s not found: %v", defaultPath, statErr)
			}
			return "", fmt.Errorf("dynamic generation failed and no default config available: %w", err)
		}
		return "", fmt.Errorf("failed to generate MIG config: %w", err)
	}

	log.Infof("Generated config: %s", DefaultGeneratedConfigFile)
	if err := createNodeConfigMap(ctx, clientset, nodeNameFlag, configYAML); err != nil {
		log.Warnf("Failed to create node ConfigMap: %v", err)
	}

	return DefaultGeneratedConfigFile, nil
}

func start(ctx context.Context, c *cli.Command) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigFlag)
	if err != nil {
		return fmt.Errorf("error building kubernetes clientcmd config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("error building kubernetes clientset from config: %w", err)
	}

	// Setup MIG configuration (generate from hardware or use custom config)
	configPath, err := setupMigConfig(ctx, clientset)
	if err != nil {
		return fmt.Errorf("failed to setup MIG config: %w", err)
	}

	// Update configFileFlag for rest of mig-manager code
	configFileFlag = configPath

	driverLibraryPath, nvidiaSMIPath, err := getPathsForCDI()
	if err != nil {
		return fmt.Errorf("failed to get paths required for cdi: %w", err)
	}

	migConfig := NewSyncableMigConfig()

	stop := ContinuouslySyncMigConfigChanges(clientset, migConfig)
	defer close(stop)

	for {
		log.Infof("Waiting for change to '%s' label", MigConfigLabel)
		value := migConfig.Get()
		log.Infof("Updating to MIG config: %s", value)
		err := migReconfigure(ctx, value, clientset, driverLibraryPath, nvidiaSMIPath)
		if err != nil {
			log.Errorf("Error: %s", err)
			continue
		}
		log.Infof("Successfully updated to MIG config: %s", value)
	}
}

// getPathsForCDI discovers the paths to libnvidia-ml.so.1 and nvidia-smi
// when required.
//
// After applying a MIG configuration but before generating a CDI spec,
// it is required to run nvidia-smi to create the nvidia-cap* device nodes.
// If driverRoot != devRoot, we must discover the paths to libnvidia-ml.so.1 and
// nvidia-smi in order to run nvidia-smi. We discover the paths here once and
// pass these as arguments to reconfigure-mig.sh
//
// Currently, driverRoot != devRoot only when devRoot='/'. Since mig-manager
// has rw access to the host rootFS (at hostRootMountFlag), reconfigure-mig.sh
// will first chroot into the host rootFS before invoking nvidia-smi, so the
// device nodes get created at '/dev' on the host.
func getPathsForCDI() (string, string, error) {
	if !cdiEnabledFlag || (driverRoot == devRoot) {
		return "", "", nil
	}

	driverRoot := root(filepath.Join(hostRootMountFlag, driverRoot))
	driverLibraryPath, err := driverRoot.getDriverLibraryPath()
	if err != nil {
		return "", "", fmt.Errorf("failed to locate driver libraries: %w", err)
	}
	// Strip the leading '/host' so that the path is relative to the host rootFS
	driverLibraryPath = filepath.Clean(strings.TrimPrefix(driverLibraryPath, hostRootMountFlag))

	nvidiaSMIPath, err := driverRoot.getNvidiaSMIPath()
	if err != nil {
		return "", "", fmt.Errorf("failed to locate nvidia-smi: %w", err)
	}
	// Strip the leading '/host' so that the path is relative to the host rootFS
	nvidiaSMIPath = filepath.Clean(strings.TrimPrefix(nvidiaSMIPath, hostRootMountFlag))

	return driverLibraryPath, nvidiaSMIPath, nil
}

func parseGPUCLientsFile(file string) (*GPUClients, error) {
	var err error
	var yamlBytes []byte

	if file == "" {
		return &GPUClients{}, nil
	}

	yamlBytes, err = os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read error: %w", err)
	}

	var clients GPUClients
	err = yaml.Unmarshal(yamlBytes, &clients)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &clients, nil
}

func migReconfigure(ctx context.Context, migConfigValue string, clientset *kubernetes.Clientset, driverLibraryPath string, nvidiaSMIPath string) error {
	gpuClients, err := parseGPUCLientsFile(gpuClientsFileFlag)
	if err != nil {
		return fmt.Errorf("error parsing host's GPU clients file: %w", err)
	}

	opts := &reconfigure.Options{
		NodeName:                   nodeNameFlag,
		MigConfigFile:              configFileFlag,
		SelectedMigConfig:          migConfigValue,
		HostRootMount:              hostRootMountFlag,
		HostNvidiaDir:              hostNvidiaDirFlag,
		HostMigManagerStateFile:    hostMigManagerStateFileFlag,
		HostGPUClientServices:      strings.Join(gpuClients.SystemdServices, ","),
		HostKubeletService:         hostKubeletSystemdServiceFlag,
		DefaultGPUClientsNamespace: defaultGPUClientsNamespaceFlag,
		WithReboot:                 withRebootFlag,
		WithShutdownHostGPUClients: withShutdownHostGPUClientsFlag,
	}

	if cdiEnabledFlag {
		opts.CDIEnabled = true
		opts.DriverRoot = driverRoot
		opts.DriverRootCtrPath = driverRootCtrPath
		opts.DevRoot = devRoot
		opts.DevRootCtrPath = devRootCtrPath
		opts.DriverLibraryPath = driverLibraryPath
		opts.NvidiaSMIPath = nvidiaSMIPath
		opts.NvidiaCDIHookPath = nvidiaCDIHookPath
	}

	migPartedBinary := []string{"nvidia-mig-parted"}
	if withShutdownHostGPUClientsFlag {
		hostMigPartedBinary, err := copyMigPartedToHost(hostRootMountFlag, hostNvidiaDirFlag, configFileFlag)
		if err != nil {
			return fmt.Errorf("failed to copy nvidia-mig-parted to host: %w", err)
		}
		migPartedBinary = strings.Split(hostMigPartedBinary, " ")
		opts.MigConfigFile = filepath.Join(hostNvidiaDirFlag, "mig-manager", "config.yaml")
	}

	rcfg, err := reconfigure.New(ctx, clientset, migPartedBinary, opts)
	if err != nil {
		return fmt.Errorf("error creating reconfigure instance: %w", err)
	}

	return rcfg.Run()
}

func ContinuouslySyncMigConfigChanges(clientset *kubernetes.Clientset, migConfig *SyncableMigConfig) chan struct{} {
	listWatch := cache.NewListWatchFromClient(
		clientset.CoreV1().RESTClient(),
		ResourceNodes,
		v1.NamespaceAll,
		fields.OneTermEqualSelector("metadata.name", nodeNameFlag),
	)

	_, controller := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: listWatch,
		ObjectType:    &v1.Node{},
		ResyncPeriod:  0,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				migConfig.Set(obj.(*v1.Node).Labels[MigConfigLabel])
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldLabel := oldObj.(*v1.Node).Labels[MigConfigLabel]
				newLabel := newObj.(*v1.Node).Labels[MigConfigLabel]
				if oldLabel != newLabel {
					migConfig.Set(newLabel)
				}
			},
		},
	})

	stop := make(chan struct{})
	go controller.Run(stop)
	return stop
}

// copyMigPartedToHost copies the "nvidia-mig-parted" binary from the container's root filesystem over to that of the host's.
// After copying the binary, it aliases the "nvidia-mig-parted" command to the binary located in the host.
// This ensures that all subsequent "nvidia-mig-parted" calls are executed from the host root filesystem.
func copyMigPartedToHost(hostRootMount, hostNvidiaDir, migConfigFile string) (string, error) {
	// Create directory
	dir := filepath.Join(hostRootMount, hostNvidiaDir, "mig-manager")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Copy the nvidia-mig-parted binary
	sourceBinary := "/usr/bin/nvidia-mig-parted" // Assuming it's in PATH
	destBinary := filepath.Join(dir, "nvidia-mig-parted")
	if err := copyFile(sourceBinary, destBinary); err != nil {
		return "", fmt.Errorf("failed to copy nvidia-mig-parted: %w", err)
	}

	// Copy the mig config file
	configDst := filepath.Join(dir, "config.yaml")
	if err := copyFile(migConfigFile, configDst); err != nil {
		return "", fmt.Errorf("failed to copy config file: %w", err)
	}

	hostMigPartedBinaryPath := filepath.Join(hostNvidiaDir, "mig-manager", "nvidia-mig-parted")
	hostMigPartedBinary := fmt.Sprintf("chroot %s %s", hostRootMount, hostMigPartedBinaryPath)

	return hostMigPartedBinary, nil
}

// copyFile is a helper method to perform a copy of file located at the source "src" over to the destination "dst"
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	if err != nil {
		return err
	}

	// Get source file info to retrieve permissions
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	// Explicitly set permissions on the destination file (in case OpenFile didn't fully apply them)
	err = os.Chmod(dst, sourceInfo.Mode().Perm()) //nolint:gosec // dst is from internal callers with known paths
	if err != nil {
		return err
	}

	return nil
}
