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
	"io/ioutil"
	"reflect"

	hooks "github.com/NVIDIA/mig-parted/api/hooks/v1"
	"github.com/NVIDIA/mig-parted/cmd/assert"
	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"

	"sigs.k8s.io/yaml"
)

var log = logrus.New()

func GetLogger() *logrus.Logger {
	return log
}

type Flags struct {
	assert.Flags
	HooksFile string
}

type Context struct {
	assert.Context
	Flags *Flags
	Hooks ApplyHooks
}

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
		&cli.StringFlag{
			Name:        "hooks-file",
			Aliases:     []string{"k"},
			Usage:       "Path to the hooks file",
			Destination: &applyFlags.HooksFile,
			EnvVars:     []string{"MIG_PARTED_HOOKS_FILE"},
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

func ParseHooksFile(f *Flags) (*hooks.Spec, error) {
	var err error
	var hooksYaml []byte

	hooksYaml, err = ioutil.ReadFile(f.HooksFile)
	if err != nil {
		return nil, fmt.Errorf("read error: %v", err)
	}

	var spec hooks.Spec
	err = yaml.Unmarshal(hooksYaml, &spec)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %v", err)
	}

	return &spec, nil
}

func (c *Context) HooksEnvsMap() hooks.EnvsMap {
	envs := make(hooks.EnvsMap)
	for _, flag := range c.Context.Command.Flags {
		fv := reflect.ValueOf(flag)
		for fv.Kind() == reflect.Ptr {
			fv = reflect.Indirect(fv)
		}

		value := fv.FieldByName("Destination")
		for value.Kind() == reflect.Ptr {
			value = reflect.Indirect(value)
		}

		for _, name := range fv.FieldByName("EnvVars").Interface().([]string) {
			envs[name] = fmt.Sprintf("%v", value)
		}
	}
	return envs
}

func applyWrapper(c *cli.Context, f *Flags) error {
	err := applyWrapperWithDefers(c, f)
	if err != nil {
		return err
	}
	fmt.Println("MIG configuration applied successfully")
	return nil
}

func applyWrapperWithDefers(c *cli.Context, f *Flags) (rerr error) {
	err := assert.CheckFlags(&f.Flags)
	if err != nil {
		cli.ShowSubcommandHelp(c)
		return err
	}

	log.Debugf("Parsing config file...")
	spec, err := assert.ParseConfigFile(&f.Flags)
	if err != nil {
		return fmt.Errorf("error parsing config file: %v", err)
	}

	log.Debugf("Selecting specific MIG config...")
	migConfig, err := assert.GetSelectedMigConfig(&f.Flags, spec)
	if err != nil {
		return fmt.Errorf("error selecting MIG config: %v", err)
	}

	hooksSpec := &hooks.Spec{}
	if f.HooksFile != "" {
		log.Debugf("Parsing Hooks file...")
		hooksSpec, err = ParseHooksFile(f)
		if err != nil {
			return fmt.Errorf("error parsing hooks file: %v", err)
		}
	}

	context := Context{
		Context: assert.Context{
			Context:   c,
			Flags:     &f.Flags,
			MigConfig: migConfig,
		},
		Flags: f,
		Hooks: &applyHooks{hooksSpec.Hooks},
	}

	log.Debugf("Running apply-start hook")
	err = context.Hooks.ApplyStart(context.HooksEnvsMap(), c.Bool("debug"))
	if err != nil {
		return fmt.Errorf("error running apply-start hook: %v", err)
	}

	defer func() {
		log.Debugf("Running apply-exit hook")
		err := context.Hooks.ApplyExit(context.HooksEnvsMap(), c.Bool("debug"))
		if rerr == nil && err != nil {
			rerr = fmt.Errorf("error running apply-exit hook: %v", err)
			return
		}
		if err != nil {
			log.Errorf("Error running apply-exit hook: %v", err)
		}
	}()

	log.Debugf("Checking current MIG mode...")
	err = assert.AssertMigMode(&context.Context)
	if err != nil {
		log.Debugf("Running pre-apply-mode hook")
		err := context.Hooks.PreApplyMode(context.HooksEnvsMap(), c.Bool("debug"))
		if err != nil {
			return fmt.Errorf("error running pre-apply-mode hook: %v", err)
		}

		log.Debugf("Applying MIG mode change...")
		err = ApplyMigMode(&context)
		if err != nil {
			return err
		}
	}

	if f.ModeOnly {
		return nil
	}

	log.Debugf("Checking current MIG device configuration...")
	err = assert.AssertMigConfig(&context.Context)
	if err != nil {
		log.Debugf("Running pre-apply-config hook")
		err := context.Hooks.PreApplyConfig(context.HooksEnvsMap(), c.Bool("debug"))
		if err != nil {
			return fmt.Errorf("error running pre-apply-config hook: %v", err)
		}

		log.Debugf("Applying MIG device configuration...")
		err = ApplyMigConfig(&context)
		if err != nil {
			return err
		}
	}

	return nil
}
