/*
 * Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMigConfigEquals(t *testing.T) {
	tests := []struct {
		name  string
		left  MigConfig
		right MigConfig
		equal bool
	}{
		{"nil and empty", nil, MigConfig{}, true},
		{"same profiles", MigConfig{"1g.5gb": 2}, MigConfig{"1g.5gb": 2}, true},
		{"zero count omitted", MigConfig{"1g.5gb": 2, "2g.10gb": 0}, MigConfig{"1g.5gb": 2}, true},
		{"zero count on both sides", MigConfig{"1g.5gb": 2, "2g.10gb": 0}, MigConfig{"1g.5gb": 2, "2g.10gb": 0}, true},
		{"different zero count profiles", MigConfig{"1g.5gb": 2, "2g.10gb": 0}, MigConfig{"1g.5gb": 2, "3g.20gb": 0}, true},
		{"different counts", MigConfig{"1g.5gb": 2}, MigConfig{"1g.5gb": 1}, false},
		{"extra positive profile", MigConfig{"1g.5gb": 2, "2g.10gb": 1}, MigConfig{"1g.5gb": 2}, false},
		{"different profiles", MigConfig{"1g.5gb": 1}, MigConfig{"2g.10gb": 1}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.equal, tc.left.Equals(tc.right))
			require.Equal(t, tc.equal, tc.right.Equals(tc.left))
		})
	}
}
