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
	"fmt"

	v1 "github.com/NVIDIA/mig-parted/api/spec/v1"
	"github.com/NVIDIA/mig-parted/pkg/mig/builder"
	"github.com/NVIDIA/mig-parted/pkg/mig/discovery"
)

// GenerateMigConfigSpec discovers MIG profiles on all GPUs and generates a config spec
func GenerateMigConfigSpec(c *Context) (*v1.Spec, error) {
	// Discover MIG profiles using the shared discovery package
	deviceProfiles, err := discovery.DiscoverMIGProfiles()
	if err != nil {
		return nil, fmt.Errorf("error discovering MIG profiles: %v", err)
	}

	// Build the config specification using shared builder
	return builder.BuildMigConfigSpec(deviceProfiles)
}
