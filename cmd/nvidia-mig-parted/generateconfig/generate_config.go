/*
 * Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
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

package generateconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"

	"github.com/NVIDIA/mig-parted/pkg/mig/builder"
)

var log = logrus.New()

func GetLogger() *logrus.Logger {
	return log
}

const (
	JSONFormat = "json"
	YAMLFormat = "yaml"
)

type Flags struct {
	OutputFile   string
	OutputFormat string
}

func BuildCommand() *cli.Command {
	// Create a flags struct to hold our flags
	generateConfigFlags := Flags{}

	// Create the 'generate-config' command
	generateConfig := cli.Command{}
	generateConfig.Name = "generate-config"
	generateConfig.Usage = "Generate MIG configuration by discovering available MIG profiles on the system"
	generateConfig.Action = func(c *cli.Context) error {
		return runGenerateConfig(c, &generateConfigFlags)
	}

	// Setup the flags for this command
	generateConfig.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "output-file",
			Aliases:     []string{"f"},
			Usage:       "Output file path (default: stdout)",
			Destination: &generateConfigFlags.OutputFile,
			Value:       "",
			EnvVars:     []string{"MIG_PARTED_OUTPUT_FILE"},
		},
		&cli.StringFlag{
			Name:        "output-format",
			Aliases:     []string{"o"},
			Usage:       "Format for the output [json | yaml]",
			Destination: &generateConfigFlags.OutputFormat,
			Value:       YAMLFormat,
			EnvVars:     []string{"MIG_PARTED_OUTPUT_FORMAT"},
		},
	}

	return &generateConfig
}

func runGenerateConfig(c *cli.Context, f *Flags) error {
	err := checkFlags(f)
	if err != nil {
		_ = cli.ShowSubcommandHelp(c)
		return err
	}

	// Determine output writer
	writer := io.Writer(os.Stdout)
	if f.OutputFile != "" {
		file, err := os.Create(f.OutputFile)
		if err != nil {
			return fmt.Errorf("error creating output file: %v", err)
		}
		defer file.Close()
		writer = file
	}

	// Generate and write output based on format
	var output []byte
	switch f.OutputFormat {
	case YAMLFormat:
		// Use GenerateConfigYAML directly - no duplicate marshaling
		output, err = builder.GenerateConfigYAML()
		if err != nil {
			return fmt.Errorf("error generating MIG config: %v", err)
		}
	case JSONFormat:
		// Need the spec object to marshal to JSON
		spec, err := builder.GenerateConfigSpec()
		if err != nil {
			return fmt.Errorf("error generating MIG config: %v", err)
		}
		output, err = json.MarshalIndent(spec, "", "  ")
		if err != nil {
			return fmt.Errorf("error marshaling MIG config to JSON: %v", err)
		}
	}

	if _, err := writer.Write(output); err != nil {
		return fmt.Errorf("error writing output: %w", err)
	}

	return nil
}

func checkFlags(f *Flags) error {
	switch f.OutputFormat {
	case JSONFormat:
	case YAMLFormat:
	default:
		return fmt.Errorf("unrecognized 'output-format': %v", f.OutputFormat)
	}
	return nil
}
