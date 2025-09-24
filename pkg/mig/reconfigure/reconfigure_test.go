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

package reconfigure

import (
	"testing"
)

func TestMaybeSetPaused(t *testing.T) {
	reconfigure := &Reconfigure{}

	tests := []struct {
		input    string
		expected string
	}{
		{"false", "false"},
		{"true", "paused-for-mig-change"},
		{"paused-for-mig-change", "paused-for-mig-change"},
		{"", "paused-for-mig-change"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := reconfigure.maybeSetPaused(tt.input)
			if result != tt.expected {
				t.Errorf("maybeSetPaused(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMaybeSetTrue(t *testing.T) {
	reconfigure := &Reconfigure{}

	tests := []struct {
		input    string
		expected string
	}{
		{"false", "false"},
		{"true", "true"},
		{"paused-for-mig-change", "true"},
		{"", "true"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := reconfigure.maybeSetTrue(tt.input)
			if result != tt.expected {
				t.Errorf("maybeSetTrue(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}
