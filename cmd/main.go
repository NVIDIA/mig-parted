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
	"os"

	"github.com/NVIDIA/mig-parted/cmd/apply"
	"github.com/NVIDIA/mig-parted/cmd/assert"
	"github.com/NVIDIA/mig-parted/cmd/checkpoint"
	"github.com/NVIDIA/mig-parted/cmd/export"
	"github.com/NVIDIA/mig-parted/cmd/util"
	log "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
)

type Flags struct {
	Debug bool
}

func main() {
	// Create a flags struct to hold our flags
	flags := Flags{}

	// Create the top-level CLI
	c := cli.NewApp()
	c.UseShortOptionHandling = true
	c.EnableBashCompletion = true
	c.Usage = "Manage MIG partitions across the full set of NVIDIA GPUs on a node"
	c.Version = "0.3.0"

	// Setup the flags for this command
	c.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:        "debug",
			Aliases:     []string{"d"},
			Usage:       "Enable debug-level logging",
			Destination: &flags.Debug,
			EnvVars:     []string{"MIG_PARTED_DEBUG"},
		},
	}

	// Register the subcommands with the top-level CLI
	c.Commands = []*cli.Command{
		apply.BuildCommand(),
		assert.BuildCommand(),
		export.BuildCommand(),
		checkpoint.BuildCommand(),
	}

	// Set log-level for all subcommands
	c.Before = func(c *cli.Context) error {
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
		checkpointLog := export.GetLogger()
		checkpointLog.SetLevel(logLevel)
		return nil
	}

	// Run the CLI
	err := c.Run(os.Args)
	if err != nil {
		log.Fatalf(util.Capitalize(err.Error()))
	}
}
