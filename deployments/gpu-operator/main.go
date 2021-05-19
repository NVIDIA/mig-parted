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
	"os/exec"
	"sync"

	log "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	ResourceNodes  = "nodes"
	MigConfigLabel = "nvidia.com/mig.config"

	DefaultReconfigureScript = "/usr/bin/reconfigure-mig.sh"
	DefaultHostRootMount     = "/host"
)

var (
	kubeconfigFlag        string
	nodeNameFlag          string
	configFileFlag        string
	reconfigureScriptFlag string
	withRebootFlag        bool
	hostRootMountFlag     string
)

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
	c := cli.NewApp()
	c.Before = validateFlags
	c.Action = start

	c.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "kubeconfig",
			Value:       "",
			Usage:       "absolute path to the kubeconfig file",
			Destination: &kubeconfigFlag,
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
			Destination: &configFileFlag,
			EnvVars:     []string{"CONFIG_FILE"},
		},
		&cli.StringFlag{
			Name:        "reconfigure-script",
			Aliases:     []string{"s"},
			Value:       DefaultReconfigureScript,
			Usage:       "script to run to do the actual MIG reconfiguration",
			Destination: &reconfigureScriptFlag,
			EnvVars:     []string{"RECONFIGURE_SCRIPT"},
		},
		&cli.StringFlag{
			Name:        "host-root-mount",
			Aliases:     []string{"m"},
			Value:       DefaultHostRootMount,
			Usage:       "target path where host root directory is mounted",
			Destination: &hostRootMountFlag,
			EnvVars:     []string{"HOST_ROOT_MOUNT"},
		},
		&cli.BoolFlag{
			Name:        "with-reboot",
			Aliases:     []string{"r"},
			Value:       false,
			Usage:       "reboot the node if changing the MIG mode fails for any reason",
			Destination: &withRebootFlag,
			EnvVars:     []string{"WITH_REBOOT"},
		},
	}

	err := c.Run(os.Args)
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func validateFlags(c *cli.Context) error {
	if nodeNameFlag == "" {
		return fmt.Errorf("invalid -n <node-name> flag: must not be empty string")
	}
	if configFileFlag == "" {
		return fmt.Errorf("invalid -f <config-file> flag: must not be empty string")
	}
	return nil
}

func start(c *cli.Context) error {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigFlag)
	if err != nil {
		return fmt.Errorf("error building kubernetes clientcmd config: %s", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("error building kubernetes clientset from config: %s", err)
	}

	migConfig := NewSyncableMigConfig()

	stop := ContinuouslySyncMigConfigChanges(clientset, migConfig)
	defer close(stop)

	for {
		log.Infof("Waiting for change to '%s' label", MigConfigLabel)
		value := migConfig.Get()
		log.Infof("Updating to MIG config: %s", value)
		err := runScript(value)
		if err != nil {
			log.Errorf("Error: %s", err)
			continue
		}
		log.Infof("Successfuly updated to MIG config: %s", value)
	}
}

func runScript(migConfigValue string) error {
	args := []string{
		"-n", nodeNameFlag,
		"-f", configFileFlag,
		"-c", migConfigValue,
		"-m", hostRootMountFlag,
	}
	if withRebootFlag {
		args = append(args, "-r")
	}
	if hostRootMountFlag != "" {
		args = append(args, "-m")
	}
	cmd := exec.Command(reconfigureScriptFlag, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ContinuouslySyncMigConfigChanges(clientset *kubernetes.Clientset, migConfig *SyncableMigConfig) chan struct{} {
	listWatch := cache.NewListWatchFromClient(
		clientset.CoreV1().RESTClient(),
		ResourceNodes,
		v1.NamespaceAll,
		fields.OneTermEqualSelector("metadata.name", nodeNameFlag),
	)

	_, controller := cache.NewInformer(
		listWatch, &v1.Node{}, 0,
		cache.ResourceEventHandlerFuncs{
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
	)

	stop := make(chan struct{})
	go controller.Run(stop)
	return stop
}
