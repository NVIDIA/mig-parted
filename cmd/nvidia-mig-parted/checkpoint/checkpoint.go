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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	checkpoint "github.com/NVIDIA/mig-parted/api/checkpoint/v1"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/util"
	"github.com/NVIDIA/mig-parted/pkg/mig/state"
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
	checkpoint.Action = func(_ context.Context, c *cli.Command) error {
		return checkpointWrapper(c, &checkpointFlags)
	}

	// Setup the flags for this command
	checkpoint.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "checkpoint-file",
			Aliases:     []string{"f"},
			Usage:       "Path to the checkpoint file",
			Destination: &checkpointFlags.CheckpointFile,
			Sources:     cli.EnvVars("MIG_PARTED_CHECKPOINT_FILE"),
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

func checkpointWrapper(c *cli.Command, f *Flags) error {
	err := CheckFlags(f)
	if err != nil {
		_ = cli.ShowSubcommandHelp(c)
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

	checkpointFile, err := os.Create(f.CheckpointFile)
	if err != nil {
		return fmt.Errorf("error creating checkpoint file: %w", err)
	}
	defer checkpointFile.Close()
	if _, err := checkpointFile.Write(j); err != nil {
		return fmt.Errorf("error writing checkpoint file: %v", err)
	}

	fmt.Println("MIG configuration checkpointed successfully")
	return nil
}
