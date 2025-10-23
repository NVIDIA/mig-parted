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
	"context"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/apply"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/assert"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/checkpoint"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/export"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/generateconfig"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/restore"
	"github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted/util"
	"github.com/NVIDIA/mig-parted/internal/info"
)

// Flags holds variables that represent the set of top level flags that can be passed to the mig-parted CLI.
type Flags struct {
	Debug bool
}

func main() {
	// Create a flags struct to hold our flags
	flags := Flags{}

	// Create the top-level CLI
	c := cli.Command{}
	c.UseShortOptionHandling = true
	c.EnableShellCompletion = true
	c.Usage = "Manage MIG partitions across the full set of NVIDIA GPUs on a node"
	c.Version = info.GetVersionString()

	// Setup the flags for this command
	c.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:        "debug",
			Aliases:     []string{"d"},
			Usage:       "Enable debug-level logging",
			Destination: &flags.Debug,
			Sources:     cli.EnvVars("MIG_PARTED_DEBUG"),
		},
	}

	// Register the subcommands with the top-level CLI
	c.Commands = []*cli.Command{
		apply.BuildCommand(),
		assert.BuildCommand(),
		export.BuildCommand(),
		generateconfig.BuildCommand(),
		checkpoint.BuildCommand(),
		restore.BuildCommand(),
	}

	// Set log-level for all subcommands
	c.Before = func(ctx context.Context, c *cli.Command) (context.Context, error) {
		logLevel := log.InfoLevel
		if flags.Debug {
			logLevel = log.DebugLevel
		}
		applyLog := apply.GetLogger()
		applyLog.SetLevel(logLevel)
		assertLog := assert.GetLogger()
		assertLog.SetLevel(logLevel)
		exportLog := export.GetLogger()
		exportLog.SetLevel(logLevel)
		generateConfigLog := generateconfig.GetLogger()
		generateConfigLog.SetLevel(logLevel)
		checkpointLog := export.GetLogger()
		checkpointLog.SetLevel(logLevel)
		restoreLog := export.GetLogger()
		restoreLog.SetLevel(logLevel)
		return ctx, nil
	}

	// Run the CLI
	err := c.Run(context.Background(), os.Args)
	if err != nil {
		log.Fatal(util.Capitalize(err.Error()))
	}
}
