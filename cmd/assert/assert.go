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

package assert

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/cmd/util"
	"github.com/NVIDIA/mig-parted/pkg/types"
	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"

	"gitlab.com/nvidia/cloud-native/go-nvlib/pkg/nvpci"

	"sigs.k8s.io/yaml"
)

var log = logrus.New()

func GetLogger() *logrus.Logger {
	return log
}

type Flags struct {
	ConfigFile     string
	SelectedConfig string
	SkipReset      bool
	ModeOnly       bool
}

type Context struct {
	*cli.Context
	Flags     *Flags
	MigConfig v1.MigConfigSpecSlice
}

func BuildCommand() *cli.Command {
	// Create a flags struct to hold our flags
	assertFlags := Flags{}

	// Create the 'assert' command
	assert := cli.Command{}
	assert.Name = "assert"
	assert.Usage = "Assert that a specific MIG configuration is currently applied to the node"
	assert.Action = func(c *cli.Context) error {
		return assertWrapper(c, &assertFlags)
	}

	// Setup the flags for this command
	assert.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "config-file",
			Aliases:     []string{"f"},
			Usage:       "Path to the configuration file",
			Destination: &assertFlags.ConfigFile,
			EnvVars:     []string{"MIG_PARTED_CONFIG_FILE"},
		},
		&cli.StringFlag{
			Name:        "selected-config",
			Aliases:     []string{"c"},
			Usage:       "The label of the mig-config from the config file to assert is applied to the node",
			Destination: &assertFlags.SelectedConfig,
			EnvVars:     []string{"MIG_PARTED_SELECTED_CONFIG"},
		},
		&cli.BoolFlag{
			Name:        "mode-only",
			Aliases:     []string{"m"},
			Usage:       "Only assert the MIG mode setting from the selected config, not the configured MIG devices",
			Destination: &assertFlags.ModeOnly,
			EnvVars:     []string{"MIG_PARTED_MODE_CHANGE_ONLY"},
		},
	}

	return &assert
}

func assertWrapper(c *cli.Context, f *Flags) error {
	err := CheckFlags(f)
	if err != nil {
		cli.ShowSubcommandHelp(c)
		return err
	}

	log.Debugf("Parsing config file...")
	spec, err := ParseConfigFile(f)
	if err != nil {
		return fmt.Errorf("error parsing config file: %v", err)
	}

	log.Debugf("Selecting specific MIG config...")
	migConfig, err := GetSelectedMigConfig(f, spec)
	if err != nil {
		return fmt.Errorf("error selecting MIG config: %v", err)
	}

	context := Context{
		Context:   c,
		Flags:     f,
		MigConfig: migConfig,
	}

	log.Debugf("Asserting MIG mode configuration...")
	err = AssertMigMode(&context)
	if err != nil {
		log.Debug(util.Capitalize(err.Error()))
		return fmt.Errorf("Assertion failure: selected configuration not currently applied")
	}

	if f.ModeOnly {
		fmt.Println("Selected MIG mode settings from configuration currently applied")
		return nil
	}

	log.Debugf("Asserting MIG device configuration...")
	err = AssertMigConfig(&context)
	if err != nil {
		log.Debug(util.Capitalize(err.Error()))
		return fmt.Errorf("Assertion failure: selected configuration not currently applied")
	}

	fmt.Println("Selected MIG configuration currently applied")
	return nil
}

func CheckFlags(f *Flags) error {
	var missing []string
	if f.ConfigFile == "" {
		missing = append(missing, "config-file")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flags '%v'", strings.Join(missing, ", "))
	}
	return nil
}

func ParseConfigFile(f *Flags) (*v1.Spec, error) {
	var err error
	var configYaml []byte

	if f.ConfigFile == "-" {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			configYaml = append(configYaml, scanner.Bytes()...)
			configYaml = append(configYaml, '\n')
		}
	} else {
		configYaml, err = ioutil.ReadFile(f.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("read error: %v", err)
		}
	}

	var spec v1.Spec
	err = yaml.Unmarshal(configYaml, &spec)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %v", err)
	}

	return &spec, nil
}

func GetSelectedMigConfig(f *Flags, spec *v1.Spec) (v1.MigConfigSpecSlice, error) {
	if len(spec.MigConfigs) > 1 && f.SelectedConfig == "" {
		return nil, fmt.Errorf("missing required flag 'selected-config' when more than one config available")
	}

	if len(spec.MigConfigs) == 1 && f.SelectedConfig == "" {
		for c := range spec.MigConfigs {
			f.SelectedConfig = c
		}
	}

	if _, exists := spec.MigConfigs[f.SelectedConfig]; !exists {
		return nil, fmt.Errorf("selected mig-config not present: %v", f.SelectedConfig)
	}

	return spec.MigConfigs[f.SelectedConfig], nil
}

func WalkSelectedMigConfigForEachGPU(migConfig v1.MigConfigSpecSlice, f func(*v1.MigConfigSpec, int, types.DeviceID) error) error {
	nvpci := nvpci.New()
	gpus, err := nvpci.GetGPUs()
	if err != nil {
		return fmt.Errorf("Error enumerating GPUs: %v", err)
	}

	for _, mc := range migConfig {
		if mc.DeviceFilter == "" {
			log.Debugf("Walking MigConfig for (devices=%v)", mc.Devices)
		} else {
			log.Debugf("Walking MigConfig for (device-filter=%v, devices=%v)", mc.DeviceFilter, mc.Devices)
		}

		for i, gpu := range gpus {
			deviceID := types.NewDeviceID(gpu.Device, gpu.Vendor)

			if !mc.MatchesDeviceFilter(deviceID) {
				continue
			}

			if !mc.MatchesDevices(i) {
				continue
			}

			log.Debugf("  GPU %v: %v", i, deviceID)

			err = f(&mc, i, deviceID)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
