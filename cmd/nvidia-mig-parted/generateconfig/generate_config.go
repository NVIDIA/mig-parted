/*
 * Copyright (c) NVIDIA CORPORATION.  All rights reserved.
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
	"fmt"
	"io"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

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
	generateConfigFlags := Flags{}

	generateConfig := cli.Command{}
	generateConfig.Name = "generate-config"
	generateConfig.Usage = "Generate MIG configuration by discovering available MIG profiles on the system"
	generateConfig.Action = func(c *cli.Context) error {
		return runGenerateConfig(c, &generateConfigFlags)
	}

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

	writer := io.Writer(os.Stdout)
	if f.OutputFile != "" {
		file, err := os.Create(f.OutputFile)
		if err != nil {
			return fmt.Errorf("error creating output file: %w", err)
		}
		defer file.Close()
		writer = file
	}

	var output []byte
	switch f.OutputFormat {
	case YAMLFormat:
		output, err = builder.GenerateConfigYAML()
		if err != nil {
			return fmt.Errorf("error generating MIG config: %w", err)
		}
	case JSONFormat:
		output, err = builder.GenerateConfigJSON()
		if err != nil {
			return fmt.Errorf("error generating MIG config: %w", err)
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
		return fmt.Errorf("unrecognized 'output-format': %s", f.OutputFormat)
	}
	return nil
}
