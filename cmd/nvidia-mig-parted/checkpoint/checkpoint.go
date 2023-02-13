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

package checkpoint

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	checkpoint "github.com/NVIDIA/mig-parted/api/checkpoint/v1"
	"github.com/NVIDIA/mig-parted/cmd/util"
	"github.com/NVIDIA/mig-parted/internal/nvml"
	"github.com/NVIDIA/mig-parted/pkg/mig/state"
	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
)

var log = logrus.New()

func GetLogger() *logrus.Logger {
	return log
}

type Flags struct {
	CheckpointFile string
}

func BuildCommand() *cli.Command {
	// Create a flags struct to hold our flags
	checkpointFlags := Flags{}

	// Create the 'checkpoint' command
	checkpoint := cli.Command{}
	checkpoint.Name = "checkpoint"
	checkpoint.Usage = "Checkpoint MIG state to a checkpoint file"
	checkpoint.Action = func(c *cli.Context) error {
		return checkpointWrapper(c, &checkpointFlags)
	}

	// Setup the flags for this command
	checkpoint.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "checkpoint-file",
			Aliases:     []string{"f"},
			Usage:       "Path to the checkpoint file",
			Destination: &checkpointFlags.CheckpointFile,
			EnvVars:     []string{"MIG_PARTED_CHECKPOINT_FILE"},
		},
	}

	return &checkpoint
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

func checkpointWrapper(c *cli.Context, f *Flags) error {
	err := CheckFlags(f)
	if err != nil {
		cli.ShowSubcommandHelp(c)
		return err
	}

	nvml := nvml.New()
	err = util.NvmlInit(nvml)
	if err != nil {
		return fmt.Errorf("error initializing NVML: %v", err)
	}
	defer util.TryNvmlShutdown(nvml)

	migState, err := state.NewMigStateManager().Fetch()
	if err != nil {
		return fmt.Errorf("error fetching MIG state: %v", err)
	}

	state := checkpoint.State{
		Version:  checkpoint.Version,
		MigState: *migState,
	}

	j, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("error marshalling MIG state to json: %v", err)
	}

	err = os.WriteFile(f.CheckpointFile, []byte(j), 0666)
	if err != nil {
		return fmt.Errorf("error writing checkpoint file: %v", err)
	}

	fmt.Println("MIG configuration checkpointed successfully")
	return nil
}
