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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"

	"github.com/NVIDIA/mig-parted/internal/info"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"sigs.k8s.io/yaml"
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
)

var nodeNameFlag string

type options struct {
	kubeconfig                 string
	configFile                 string
	reconfigureScript          string
	withReboot                 bool
	withShutdownHostGPUClients bool
	gpuClientsFile             string
	hostRootMount              string
	hostNvidiaDir              string
	hostMigManagerStateFile    string
	hostKubeletSystemdService  string
	defaultGPUClientsNamespace string

	cdiEnabled        bool
	driverRoot        string
	driverRootCtrPath string
	devRoot           string
	devRootCtrPath    string
	nvidiaCDIHookPath string
}

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
	o := &options{}

	c := cli.NewApp()
	c.Before = func(ctx *cli.Context) error {
		return validateFlags(ctx, o)
	}
	c.Action = func(ctx *cli.Context) error {
		return start(ctx, o)
	}
	c.Version = info.GetVersionString()

	c.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "kubeconfig",
			Value:       "",
			Usage:       "absolute path to the kubeconfig file",
			Destination: &o.kubeconfig,
			EnvVars:     []string{"KUBECONFIG"},
		},
		&cli.StringFlag{
			Name:        "node-name",
			Aliases:     []string{"n"},
			Value:       "",
			Usage:       "the name of the node to watch for label changes on",
			Destination: &nodeNameFlag,
			EnvVars:     []string{"NODE_NAME"},
		},
		&cli.StringFlag{
			Name:        "config-file",
			Aliases:     []string{"f"},
			Value:       "",
			Usage:       "the path to the MIG parted configuration file",
			Destination: &o.configFile,
			EnvVars:     []string{"CONFIG_FILE"},
		},
		&cli.StringFlag{
			Name:        "reconfigure-script",
			Aliases:     []string{"s"},
			Value:       DefaultReconfigureScript,
			Usage:       "script to run to do the actual MIG reconfiguration",
			Destination: &o.reconfigureScript,
			EnvVars:     []string{"RECONFIGURE_SCRIPT"},
		},
		&cli.StringFlag{
			Name:        "host-root-mount",
			Aliases:     []string{"m"},
			Value:       DefaultHostRootMount,
			Usage:       "container path where host root directory is mounted",
			Destination: &o.hostRootMount,
			EnvVars:     []string{"HOST_ROOT_MOUNT"},
		},
		&cli.StringFlag{
			Name:        "host-nvidia-dir",
			Aliases:     []string{"i"},
			Value:       DefaultHostNvidiaDir,
			Usage:       "host path of the directory where NVIDIA managed software directory is typically located",
			Destination: &o.hostNvidiaDir,
			EnvVars:     []string{"HOST_NVIDIA_DIR"},
		},
		&cli.StringFlag{
			Name:        "host-mig-manager-state-file",
			Aliases:     []string{"o"},
			Value:       DefaultHostMigManagerStateFile,
			Usage:       "host path where the host's systemd mig-manager state file is located",
			Destination: &o.hostMigManagerStateFile,
			EnvVars:     []string{"HOST_MIG_MANAGER_STATE_FILE"},
		},
		&cli.StringFlag{
			Name:        "host-kubelet-systemd-service",
			Aliases:     []string{"k"},
			Value:       DefaultHostKubeletSystemdService,
			Usage:       "name of the host's 'kubelet' systemd service which may need to be shutdown/restarted across a MIG mode reconfiguration",
			Destination: &o.hostKubeletSystemdService,
			EnvVars:     []string{"HOST_KUBELET_SYSTEMD_SERVICE"},
		},
		&cli.StringFlag{
			Name:        "gpu-clients-file",
			Aliases:     []string{"g"},
			Value:       "",
			Usage:       "the path to the file listing the GPU clients that need to be shutdown across a MIG configuration",
			Destination: &o.gpuClientsFile,
			EnvVars:     []string{"GPU_CLIENTS_FILE"},
		},
		&cli.BoolFlag{
			Name:        "with-reboot",
			Aliases:     []string{"r"},
			Value:       false,
			Usage:       "reboot the node if changing the MIG mode fails for any reason",
			Destination: &o.withReboot,
			EnvVars:     []string{"WITH_REBOOT"},
		},
		&cli.BoolFlag{
			Name:        "with-shutdown-host-gpu-clients",
			Aliases:     []string{"d"},
			Value:       false,
			Usage:       "shutdown/restart any required host GPU clients across a MIG configuration",
			Destination: &o.withShutdownHostGPUClients,
			EnvVars:     []string{"WITH_SHUTDOWN_HOST_GPU_CLIENTS"},
		},
		&cli.StringFlag{
			Name:        "default-gpu-clients-namespace",
			Aliases:     []string{"p"},
			Value:       DefaultGPUClientsNamespace,
			Usage:       "Default name of the Kubernetes namespace in which the GPU client Pods are installed in",
			Destination: &o.defaultGPUClientsNamespace,
			EnvVars:     []string{"DEFAULT_GPU_CLIENTS_NAMESPACE"},
		},
		&cli.StringFlag{
			Name:        "nvidia-driver-root",
			Aliases:     []string{"driver-root", "t"},
			Value:       DefaultNvidiaDriverRoot,
			Usage:       "Root path to the NVIDIA driver installation. Only used if --cdi-enabled is set.",
			Destination: &o.driverRoot,
			EnvVars:     []string{"NVIDIA_DRIVER_ROOT", "DRIVER_ROOT"},
		},
		&cli.StringFlag{
			Name:        "driver-root-ctr-path",
			Aliases:     []string{"a"},
			Value:       DefaultDriverRootCtrPath,
			Usage:       "Root path to the NVIDIA driver installation mounted in the container. Only used if --cdi-enabled is set.",
			Destination: &o.driverRootCtrPath,
			EnvVars:     []string{"DRIVER_ROOT_CTR_PATH"},
		},
		&cli.BoolFlag{
			Name:        "cdi-enabled",
			Usage:       "Enable CDI support",
			Destination: &o.cdiEnabled,
			EnvVars:     []string{"CDI_ENABLED"},
		},
		&cli.StringFlag{
			Name:        "dev-root",
			Aliases:     []string{"b"},
			Value:       "",
			Usage:       "Root path to the NVIDIA device nodes. Only used if --cdi-enabled is set.",
			Destination: &o.devRoot,
			EnvVars:     []string{"NVIDIA_DEV_ROOT"},
		},
		&cli.StringFlag{
			Name:        "dev-root-ctr-path",
			Aliases:     []string{"j"},
			Value:       "",
			Usage:       "Root path to the NVIDIA device nodes mounted in the container. Only used if --cdi-enabled is set.",
			Destination: &o.devRootCtrPath,
			EnvVars:     []string{"DEV_ROOT_CTR_PATH"},
		},
		&cli.StringFlag{
			Name:        "nvidia-cdi-hook-path",
			Value:       DefaultNvidiaCDIHookPath,
			Usage:       "Path to nvidia-cdi-hook binary on the host.",
			Destination: &o.nvidiaCDIHookPath,
			EnvVars:     []string{"NVIDIA_CDI_HOOK_PATH"},
		},
	}

	err := c.Run(os.Args)
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func validateFlags(c *cli.Context, o *options) error {
	if nodeNameFlag == "" {
		return fmt.Errorf("invalid -n <node-name> flag: must not be empty string")
	}
	if o.configFile == "" {
		return fmt.Errorf("invalid -f <config-file> flag: must not be empty string")
	}
	return nil
}

func start(c *cli.Context, o *options) error {
	config, err := clientcmd.BuildConfigFromFlags("", o.kubeconfig)
	if err != nil {
		return fmt.Errorf("error building kubernetes clientcmd config: %s", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("error building kubernetes clientset from config: %s", err)
	}

	driverLibraryPath, nvidiaSMIPath, err := getPathsForCDI(o)
	if err != nil {
		return fmt.Errorf("failed to get paths required for cdi: %w", err)
	}

	manager := &migManager{
		clientset:         clientset,
		migConfig:         NewSyncableMigConfig(),
		nodeName:          nodeNameFlag,
		driverLibraryPath: driverLibraryPath,
		nvidiaSMIPath:     nvidiaSMIPath,
	}

	stop := manager.ContinuouslySyncMigConfigChanges()
	defer close(stop)

	for {
		log.Infof("Waiting for change to '%s' label", MigConfigLabel)
		migConfigLabelValue := manager.Get()
		log.Infof("Updating to MIG config: %s", migConfigLabelValue)
		if err := manager.Reconfigure(o, migConfigLabelValue); err != nil {
			log.Errorf("Error: %s", err)
			continue
		}
		log.Infof("Successfully updated to MIG config: %s", migConfigLabelValue)
	}
}

// A migManger is responsible for watching a particular label and triggering a
// reconfiguration if the value change.
type migManager struct {
	clientset         *kubernetes.Clientset
	migConfig         *SyncableMigConfig
	nodeName          string
	driverLibraryPath string
	nvidiaSMIPath     string
}

func (m *migManager) Get() string {
	return m.migConfig.Get()
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
// has rw access to the host rootFS (at hostRootMount), reconfigure-mig.sh
// will first chroot into the host rootFS before invoking nvidia-smi, so the
// device nodes get created at '/dev' on the host.
func getPathsForCDI(o *options) (string, string, error) {
	if !o.cdiEnabled || (o.driverRoot == o.devRoot) {
		return "", "", nil
	}

	driverRoot := root(filepath.Join(o.hostRootMount, o.driverRoot))
	driverLibraryPath, err := driverRoot.getDriverLibraryPath()
	if err != nil {
		return "", "", fmt.Errorf("failed to locate driver libraries: %w", err)
	}
	// Strip the leading '/host' so that the path is relative to the host rootFS
	driverLibraryPath = filepath.Clean(strings.TrimPrefix(driverLibraryPath, o.hostRootMount))

	nvidiaSMIPath, err := driverRoot.getNvidiaSMIPath()
	if err != nil {
		return "", "", fmt.Errorf("failed to locate nvidia-smi: %w", err)
	}
	// Strip the leading '/host' so that the path is relative to the host rootFS
	nvidiaSMIPath = filepath.Clean(strings.TrimPrefix(nvidiaSMIPath, o.hostRootMount))

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
		return nil, fmt.Errorf("read error: %v", err)
	}

	var clients GPUClients
	err = yaml.Unmarshal(yamlBytes, &clients)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %v", err)
	}

	return &clients, nil
}

func (m *migManager) Reconfigure(o *options, migConfigValue string) error {
	gpuClients, err := parseGPUCLientsFile(o.gpuClientsFile)
	if err != nil {
		return fmt.Errorf("error parsing host's GPU clients file: %s", err)
	}

	// TODO: Use functional options.
	opts := &reconfigureMIGOptions{
		NodeName:                   m.nodeName,
		MIGPartedConfigFile:        o.configFile,
		SelectedMIGConfig:          migConfigValue,
		HostRootMount:              o.hostRootMount,
		HostMIGManagerStateFile:    o.hostMigManagerStateFile,
		HostGPUClientServices:      gpuClients.SystemdServices,
		HostKubeletService:         o.hostKubeletSystemdService,
		GPUClientsNamespace:        o.defaultGPUClientsNamespace,
		ConfigStateLabel:           "nvidia.com/mig.config.state",
		WithReboot:                 o.withReboot,
		WithShutdownHostGPUClients: o.withShutdownHostGPUClients,

		DriverLibraryPath: m.driverLibraryPath,

		DriverRoot:        o.driverRoot,
		DriverRootCtrPath: o.driverRootCtrPath,
		DevRoot:           o.devRoot,
		DevRootCtrPath:    o.devRootCtrPath,

		CDIEnabled:        o.cdiEnabled,
		NVIDIASMIPath:     m.nvidiaSMIPath,
		NVIDIACDIHookPath: o.nvidiaCDIHookPath,
	}

	return reconfigureMIG(m.clientset, opts)
}

func (m *migManager) ContinuouslySyncMigConfigChanges() chan struct{} {
	listWatch := cache.NewListWatchFromClient(
		m.clientset.CoreV1().RESTClient(),
		ResourceNodes,
		v1.NamespaceAll,
		fields.OneTermEqualSelector("metadata.name", m.nodeName),
	)

	_, controller := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: listWatch,
		ObjectType:    &v1.Node{},
		ResyncPeriod:  0,
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				m.migConfig.Set(obj.(*v1.Node).Labels[MigConfigLabel])
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldLabel := oldObj.(*v1.Node).Labels[MigConfigLabel]
				newLabel := newObj.(*v1.Node).Labels[MigConfigLabel]
				if oldLabel != newLabel {
					m.migConfig.Set(newLabel)
				}
			},
		},
	})

	stop := make(chan struct{})
	go controller.Run(stop)
	return stop
}
