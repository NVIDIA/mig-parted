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
	"fmt"

	"github.com/NVIDIA/mig-parted/cmd/apply"
	"github.com/NVIDIA/mig-parted/cmd/util"
	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
)

var log = logrus.New()

func GetLogger() *logrus.Logger {
	return log
}

type Flags = apply.Flags
type Context = apply.Context

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
			Usage:       "Only assert the MIG mode setting from the config, not the configured MIG devices",
			Destination: &assertFlags.ModeOnly,
			EnvVars:     []string{"MIG_PARTED_MODE_CHANGE_ONLY"},
		},
	}

	return &assert
}

func assertWrapper(c *cli.Context, f *Flags) error {
	err := apply.CheckFlags(f)
	if err != nil {
		cli.ShowSubcommandHelp(c)
		return err
	}

	log.Debugf("Parsing config file...")
	spec, err := apply.ParseConfigFile(f)
	if err != nil {
		return fmt.Errorf("error parsing config file: %v", err)
	}

	log.Debugf("Selecting specific MIG config...")
	migConfig, err := apply.GetSelectedMigConfig(f, spec)
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

	if !f.ModeOnly {
		log.Debugf("Asserting MIG device configuration...")
		err = AssertMigConfig(&context)
		if err != nil {
			log.Debug(util.Capitalize(err.Error()))
			return fmt.Errorf("Assertion failure: selected configuration not currently applied")
		}
	}

	fmt.Println("Selected MIG configuration currently applied")
	return nil
}
