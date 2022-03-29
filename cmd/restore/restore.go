/*
 * Copyright (c) 2022, NVIDIA CORPORATION.  All rights reserved.
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

package restore

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	checkpoint "github.com/NVIDIA/mig-parted/api/checkpoint/v1"
	hooks "github.com/NVIDIA/mig-parted/api/hooks/v1"
	"github.com/NVIDIA/mig-parted/cmd/apply"
	"github.com/NVIDIA/mig-parted/pkg/mig/state"
	"github.com/NVIDIA/mig-parted/pkg/types"
	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
)

var log = logrus.New()

func GetLogger() *logrus.Logger {
	return log
}

type Flags struct {
	CheckpointFile string
	HooksFile      string
	ModeOnly       bool
}

type Context struct {
	*cli.Context
	Flags           *Flags
	Hooks           apply.ApplyHooks
	MigState        *types.MigState
	MigStateManager state.Manager
}

func BuildCommand() *cli.Command {
	// Create a flags struct to hold our flags
	restoreFlags := Flags{}

	// Create the 'restore' command
	restore := cli.Command{}
	restore.Name = "restore"
	restore.Usage = "Restore MIG state from a checkpoint file"
	restore.Action = func(c *cli.Context) error {
		return restoreWrapper(c, &restoreFlags)
	}

	// Setup the flags for this command
	restore.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "checkpoint-file",
			Aliases:     []string{"f"},
			Usage:       "Path to the checkpoint file",
			Destination: &restoreFlags.CheckpointFile,
			EnvVars:     []string{"MIG_PARTED_CHECKPOINT_FILE"},
		},
		&cli.StringFlag{
			Name:        "hooks-file",
			Aliases:     []string{"k"},
			Usage:       "Path to the hooks file",
			Destination: &restoreFlags.HooksFile,
			EnvVars:     []string{"MIG_PARTED_HOOKS_FILE"},
		},
		&cli.BoolFlag{
			Name:        "mode-only",
			Aliases:     []string{"m"},
			Usage:       "Only change the MIG enabled setting from the checkpoint file, not configure any MIG devices",
			Destination: &restoreFlags.ModeOnly,
			EnvVars:     []string{"MIG_PARTED_MODE_CHANGE_ONLY"},
		},
	}

	return &restore
}

func CheckFlags(f *Flags) error {
	var missing []string
	if f.CheckpointFile == "" {
		missing = append(missing, "checkpoint-file")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flags '%v'", strings.Join(missing, ", "))
	}

	return nil
}

func ParseCheckpointFile(f *Flags) (*checkpoint.State, error) {
	checkpointJson, err := os.ReadFile(f.CheckpointFile)
	if err != nil {
		return nil, fmt.Errorf("read error: %v", err)
	}

	var state checkpoint.State
	err = json.Unmarshal(checkpointJson, &state)
	if err != nil {
		return nil, fmt.Errorf("unmarshal error: %v", err)
	}

	return &state, nil
}

func (c *Context) AssertMigMode() error {
	current, err := c.MigStateManager.Fetch()
	if err != nil {
		return fmt.Errorf("error fetching MIG state: %v", err)
	}
	for i := range c.MigState.Devices {
		if current.Devices[i].MigMode != c.MigState.Devices[i].MigMode {
			return fmt.Errorf("current mode different than mode being asserted")
		}
	}
	return nil
}

func (c *Context) AssertMigConfig() error {
	current, err := c.MigStateManager.Fetch()
	if err != nil {
		return fmt.Errorf("error fetching MIG state: %v\n", err)
	}
	if !reflect.DeepEqual(current, c.MigState) {
		return fmt.Errorf("checkpoint contents do not match the current MIG state")
	}
	return nil
}

func (c *Context) ApplyMigMode() error {
	return c.MigStateManager.RestoreMode(c.MigState)
}

func (c *Context) ApplyMigConfig() error {
	return c.MigStateManager.RestoreConfig(c.MigState)
}

func restoreWrapper(c *cli.Context, f *Flags) error {
	err := CheckFlags(f)
	if err != nil {
		cli.ShowSubcommandHelp(c)
		return err
	}

	log.Debugf("Parsing checkpoint file...")
	checkpoint, err := ParseCheckpointFile(f)
	if err != nil {
		return fmt.Errorf("error parsing checkpoint file: %v", err)
	}

	hooksSpec := &hooks.Spec{}
	if f.HooksFile != "" {
		log.Debugf("Parsing Hooks file...")
		hooksSpec, err = apply.ParseHooksFile(f.HooksFile)
		if err != nil {
			return fmt.Errorf("error parsing hooks file: %v", err)
		}
	}

	context := Context{
		Context:         c,
		Flags:           f,
		Hooks:           apply.NewApplyHooks(hooksSpec.Hooks),
		MigState:        &checkpoint.MigState,
		MigStateManager: state.NewMigStateManager(),
	}

	err = apply.ApplyMigConfigWithHooks(log, c, f.ModeOnly, context.Hooks, &context)
	if err != nil {
		return fmt.Errorf("error applying MIG configuration with hooks: %v", err)
	}

	fmt.Println("MIG configuration restored successfully")
	return nil
}
