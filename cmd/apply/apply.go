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

package apply

import (
	"fmt"

	"github.com/NVIDIA/mig-parted/cmd/assert"
	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
)

var log = logrus.New()

func GetLogger() *logrus.Logger {
	return log
}

type Flags = assert.Flags
type Context = assert.Context

func BuildCommand() *cli.Command {
	// Create a flags struct to hold our flags
	applyFlags := Flags{}

	// Create the 'apply' command
	apply := cli.Command{}
	apply.Name = "apply"
	apply.Usage = "Apply changes (if necessary) for a specific MIG configuration from a configuration file"
	apply.Action = func(c *cli.Context) error {
		return applyWrapper(c, &applyFlags)
	}

	// Setup the flags for this command
	apply.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "config-file",
			Aliases:     []string{"f"},
			Usage:       "Path to the configuration file",
			Destination: &applyFlags.ConfigFile,
			EnvVars:     []string{"MIG_PARTED_CONFIG_FILE"},
		},
		&cli.StringFlag{
			Name:        "selected-config",
			Aliases:     []string{"c"},
			Usage:       "The label of the mig-config from the config file to apply to the node",
			Destination: &applyFlags.SelectedConfig,
			EnvVars:     []string{"MIG_PARTED_SELECTED_CONFIG"},
		},
		&cli.BoolFlag{
			Name:        "skip-reset",
			Aliases:     []string{"s"},
			Usage:       "Skip the GPU reset operation after applying the desired MIG mode to all GPUs",
			Destination: &applyFlags.SkipReset,
			EnvVars:     []string{"MIG_PARTED_SKIP_RESET"},
		},
		&cli.BoolFlag{
			Name:        "mode-only",
			Aliases:     []string{"m"},
			Usage:       "Only change the MIG enabled setting from the config, not configure any MIG devices",
			Destination: &applyFlags.ModeOnly,
			EnvVars:     []string{"MIG_PARTED_MODE_CHANGE_ONLY"},
		},
	}

	return &apply
}

func applyWrapper(c *cli.Context, f *Flags) error {
	err := assert.CheckFlags(f)
	if err != nil {
		cli.ShowSubcommandHelp(c)
		return err
	}

	log.Debugf("Parsing config file...")
	spec, err := assert.ParseConfigFile(f)
	if err != nil {
		return fmt.Errorf("error parsing config file: %v", err)
	}

	log.Debugf("Selecting specific MIG config...")
	migConfig, err := assert.GetSelectedMigConfig(f, spec)
	if err != nil {
		return fmt.Errorf("error selecting MIG config: %v", err)
	}

	context := Context{
		Context:   c,
		Flags:     f,
		MigConfig: migConfig,
	}

	log.Debugf("Applying MIG mode change...")
	err = ApplyMigMode(&context)
	if err != nil {
		return err
	}

	if !f.ModeOnly {
		log.Debugf("Applying MIG device configuration...")
		err = ApplyMigConfig(&context)
		if err != nil {
			return err
		}
	}

	fmt.Println("MIG configuration applied successfully")
	return nil
}
