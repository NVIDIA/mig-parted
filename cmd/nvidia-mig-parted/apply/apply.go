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
	"os"
	"reflect"

	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	hooks "github.com/NVIDIA/mig-parted/api/hooks/v1"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/assert"

	"sigs.k8s.io/yaml"
)

var log = logrus.New()

// GetLogger returns the 'logrus.Logger' instance used by this package.
func GetLogger() *logrus.Logger {
	return log
}

// Flags holds variables that represent the set of flags that can be passed to the 'apply' subcommand.
type Flags struct {
	assert.Flags
	HooksFile string
}

// Context holds the state we want to pass around between functions associated with the 'apply' subcommand.
type Context struct {
	assert.Context
	Flags *Flags
}

// MigConfigApplier is an interface representing the set of functions required to "Apply" a MIG configuration to a node.
type MigConfigApplier interface {
	AssertMigMode() error
	ApplyMigMode() error
	AssertMigConfig() error
	ApplyMigConfig() error
}

// BuildCommand builds the 'apply' subcommand for injection into the main mig-parted CLI.
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

// CheckFlags ensures that any required flags are provided and ensures they are well-formed.
func CheckFlags(f *Flags) error {
	return assert.CheckFlags(&f.Flags)
}

// ParseHooksFile parses a hoosk file and unmarshals it into a 'hooks.Spec'.
func ParseHooksFile(hooksFile string) (*hooks.Spec, error) {
	var err error
	var hooksYaml []byte

	hooksYaml, err = os.ReadFile(hooksFile)
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

// GetHooksEnvsMap builds a 'hooks.EnvsMap' from the set of environment variables set when the CLI was envoked by the user.
// These environment variables are then made available to all hooks when thex are executed later on.
func GetHooksEnvsMap(c *cli.Context) hooks.EnvsMap {
	envs := make(hooks.EnvsMap)
	for _, flag := range c.Command.Flags {
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

// AssertMigMode reuses calls from the 'assert' subcommand to ensures that the MIG mode settings of a given MIG config are currently applied.
// The 'MigConfig'  being checked is embedded in the 'Context' struct itself.
func (c *Context) AssertMigMode() error {
	return assert.AssertMigMode(&c.Context)
}

// ApplyMigMode applies the MIG mode settings of the config embedded in the 'Context' to the set of GPUs on the node.
func (c *Context) ApplyMigMode() error {
	return ApplyMigMode(c)
}

// AssertMigMode reuses calls from the 'assert' subcommand to ensures that all MIG settings of a given MIG config are currently applied.
// The 'MigConfig'  being checked is embedded in the 'Context' struct itself.
func (c *Context) AssertMigConfig() error {
	return assert.AssertMigConfig(&c.Context)
}

// ApplyMigConfig applies the full MIG config embedded in the 'Context' to the set of GPUs on the node.
func (c *Context) ApplyMigConfig() error {
	return ApplyMigConfig(c)
}

func applyWrapper(c *cli.Context, f *Flags) error {
	err := CheckFlags(f)
	if err != nil {
		_ = cli.ShowSubcommandHelp(c)
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
		hooksSpec, err = ParseHooksFile(f.HooksFile)
		if err != nil {
			return fmt.Errorf("error parsing hooks file: %v", err)
		}
	}

	hooks := NewApplyHooks(hooksSpec.Hooks)

	context := Context{
		Flags: f,
		Context: assert.Context{
			Context:   c,
			Flags:     &f.Flags,
			MigConfig: migConfig,
			Nvml:      nvml.New(),
		},
	}

	err = ApplyMigConfigWithHooks(log, c, f.ModeOnly, hooks, &context)
	if err != nil {
		return fmt.Errorf("error applying MIG configuration with hooks: %v", err)
	}

	fmt.Println("MIG configuration applied successfully")
	return nil
}

// ApplyMigConfigWithHooks orchestrates the calls of a 'MigConfigApplier' between a set of 'ApplyHooks' to the set MIG configuration of a node.
// If 'modeOnly' is 'true', then only the MIG mode settings embedded in the 'Context' are applied.
func ApplyMigConfigWithHooks(logger *logrus.Logger, context *cli.Context, modeOnly bool, hooks ApplyHooks, applier MigConfigApplier) (rerr error) {
	logger.Debugf("Running apply-start hook")
	err := hooks.ApplyStart(GetHooksEnvsMap(context), context.Bool("debug"))
	if err != nil {
		return fmt.Errorf("error running apply-start hook: %v", err)
	}

	defer func() {
		logger.Debugf("Running apply-exit hook")
		err := hooks.ApplyExit(GetHooksEnvsMap(context), context.Bool("debug"))
		if rerr == nil && err != nil {
			rerr = fmt.Errorf("error running apply-exit hook: %v", err)
			return
		}
		if err != nil {
			logger.Errorf("Error running apply-exit hook: %v", err)
		}
	}()

	logger.Debugf("Checking current MIG mode...")
	err = applier.AssertMigMode()
	if err != nil {
		logger.Debugf("Running pre-apply-mode hook")
		err := hooks.PreApplyMode(GetHooksEnvsMap(context), context.Bool("debug"))
		if err != nil {
			return fmt.Errorf("error running pre-apply-mode hook: %v", err)
		}

		logger.Debugf("Applying MIG mode change...")
		err = applier.ApplyMigMode()
		if err != nil {
			return err
		}
	}

	if modeOnly {
		return nil
	}

	logger.Debugf("Checking current MIG device configuration...")
	err = applier.AssertMigConfig()
	if err != nil {
		logger.Debugf("Running pre-apply-config hook")
		err := hooks.PreApplyConfig(GetHooksEnvsMap(context), context.Bool("debug"))
		if err != nil {
			return fmt.Errorf("error running pre-apply-config hook: %v", err)
		}

		logger.Debugf("Applying MIG device configuration...")
		err = applier.ApplyMigConfig()
		if err != nil {
			return err
		}
	}

	return nil
}
