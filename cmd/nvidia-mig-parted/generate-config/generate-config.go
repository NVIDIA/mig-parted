/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
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

	v1 "github.com/NVIDIA/mig-parted/api/spec/v1"

	yaml "gopkg.in/yaml.v2"
)

var log = logrus.New()

func GetLogger() *logrus.Logger {
	return log
}

const (
	JSONFormat         = "json"
	YAMLFormat         = "yaml"
	DefaultConfigLabel = "discovered"
)

type Flags struct {
	OutputFile   string
	OutputFormat string
	ConfigLabel  string
}

type Context struct {
	*cli.Context
	Flags *Flags
}

func BuildCommand() *cli.Command {
	// Create a flags struct to hold our flags
	generateConfigFlags := Flags{}

	// Create the 'generate-config' command
	generateConfig := cli.Command{}
	generateConfig.Name = "generate-config"
	generateConfig.Usage = "Generate MIG configuration by discovering available MIG profiles on the system"
	generateConfig.Action = func(c *cli.Context) error {
		return generateConfigWrapper(c, &generateConfigFlags)
	}

	// Setup the flags for this command
	generateConfig.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "output-file",
			Aliases:     []string{"o"},
			Usage:       "Output file path (default: stdout)",
			Destination: &generateConfigFlags.OutputFile,
			Value:       "",
			EnvVars:     []string{"MIG_PARTED_OUTPUT_FILE"},
		},
		&cli.StringFlag{
			Name:        "output-format",
			Aliases:     []string{"f"},
			Usage:       "Format for the output [json | yaml]",
			Destination: &generateConfigFlags.OutputFormat,
			Value:       YAMLFormat,
			EnvVars:     []string{"MIG_PARTED_OUTPUT_FORMAT"},
		},
		&cli.StringFlag{
			Name:        "config-label",
			Aliases:     []string{"l"},
			Usage:       "Label prefix to apply to generated configs",
			Destination: &generateConfigFlags.ConfigLabel,
			Value:       DefaultConfigLabel,
			EnvVars:     []string{"MIG_PARTED_CONFIG_LABEL"},
		},
	}

	return &generateConfig
}

func generateConfigWrapper(c *cli.Context, f *Flags) error {
	err := CheckFlags(f)
	if err != nil {
		_ = cli.ShowSubcommandHelp(c)
		return err
	}

	context := Context{
		Context: c,
		Flags:   f,
	}

	spec, err := GenerateMigConfigSpec(&context)
	if err != nil {
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

	err = WriteOutput(writer, spec, f)
	if err != nil {
		return err
	}

	return nil
}

func CheckFlags(f *Flags) error {
	switch f.OutputFormat {
	case JSONFormat:
	case YAMLFormat:
	default:
		return fmt.Errorf("unrecognized 'output-format': %v", f.OutputFormat)
	}
	return nil
}

func WriteOutput(w io.Writer, spec *v1.Spec, f *Flags) error {
	switch f.OutputFormat {
	case YAMLFormat:
		output, err := yaml.Marshal(spec)
		if err != nil {
			return fmt.Errorf("error marshaling MIG config to YAML: %v", err)
		}
		if _, err := w.Write(output); err != nil {
			return fmt.Errorf("error writing YAML output: %w", err)
		}
	case JSONFormat:
		output, err := json.MarshalIndent(spec, "", "  ")
		if err != nil {
			return fmt.Errorf("error marshaling MIG config to JSON: %v", err)
		}
		if _, err := w.Write(output); err != nil {
			return fmt.Errorf("error writing JSON output: %w", err)
		}
	}
	return nil
}
