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

package v1

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func captureOutput(f func() error) (string, error) {
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}

	stdout := os.Stdout
	stderr := os.Stderr
	defer func() {
		os.Stdout = stdout
		os.Stderr = stderr
	}()

	os.Stdout = writer
	os.Stderr = writer

	out := make(chan string)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		var buf bytes.Buffer
		wg.Done()
		io.Copy(&buf, reader)
		out <- buf.String()
	}()

	wg.Wait()
	err = f()
	writer.Close()

	return <-out, err
}

func TestMarshallUnmarshall(t *testing.T) {
	spec := Spec{
		Version: "v1",
		Hooks: HooksMap{
			"hook0": []HookSpec{
				{
					Workdir: "/wherever0",
					Command: "whatever0",
					Args:    []string{"a0", "a1"},
					Envs: EnvsMap{
						"env0": "val0",
						"env1": "val1",
					},
				},
				{
					Workdir: "/wherever1",
					Command: "whatever1",
					Args:    []string{"a0", "a1"},
					Envs: EnvsMap{
						"env0": "val0",
						"env1": "val1",
					},
				},
			},
			"hook1": []HookSpec{
				{
					Workdir: "/wherever0",
					Command: "whatever0",
					Args:    []string{"a0", "a1"},
					Envs: EnvsMap{
						"env0": "val0",
						"env1": "val1",
					},
				},
				{
					Workdir: "/wherever1",
					Command: "whatever1",
					Args:    []string{"a0", "a1"},
					Envs: EnvsMap{
						"env0": "val0",
						"env1": "val1",
					},
				},
			},
		},
	}

	y, err := yaml.Marshal(spec)
	require.Nil(t, err, "Unexpected failure yaml.Marshal")

	s := Spec{}
	err = yaml.Unmarshal(y, &s)
	require.Nil(t, err, "Unexpected failure yaml.Unmarshal")
	require.Equal(t, spec, s)
}

func TestRunHooks(t *testing.T) {
	testCases := []struct {
		Description     string
		Hook            HookSpec
		expectedOutput  string
		expectedFailure bool
	}{
		{
			"Echo Hello",
			HookSpec{
				Command: "/bin/sh",
				Args:    []string{"-c", "echo Hello"},
			},
			"Hello\n",
			false,
		},
		{
			"Nonexistent Command",
			HookSpec{
				Command: "/doesnotexist",
			},
			"",
			true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			output, err := captureOutput(func() error {
				return tc.Hook.Run(EnvsMap{}, true)
			})
			if !tc.expectedFailure {
				require.Nil(t, err, "Unexpected failure Hook.Run")
				require.Equal(t, tc.expectedOutput, output)
			} else {
				require.NotNil(t, err, "Unexpected success Hook.Run")
			}
		})
	}
}
