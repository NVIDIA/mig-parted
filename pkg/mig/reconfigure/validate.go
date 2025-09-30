/**
# SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package reconfigure

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
)

var (
	systemdServicePrefixPattern = regexp.MustCompile(`^[a-zA-Z0-9:._\\-]+\.(service|socket|device|mount|automount|swap|target|path|timer|slice|scope)$`)
)

// Validate the MIG reconfiguration options.
func (o *reconfigureMIGOptions) Validate() error {
	if o.clientset == nil {
		return fmt.Errorf("a k8s ClientSet must be specified")
	}
	validate := validator.New(validator.WithRequiredStructEnabled())

	err := validate.RegisterValidation("systemd_service_name", validateSystemdServiceName)
	if err != nil {
		return fmt.Errorf("unable to register systemd service name validator: %w", err)
	}
	return validate.Struct(o)
}

// validateSystemdServiceName validates a systemd service name according to systemd naming rules.
// The unit name prefix must consist of one or more valid characters (ASCII letters, digits, ":", "-", "_", ".", and "\").
// The total length of the unit name including the suffix must not exceed 255 characters.
// The unit type suffix must be one of ".service", ".socket", ".device", ".mount", ".automount", ".swap", ".target", ".path", ".timer", ".slice", or ".scope".
// Source: https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html
func validateSystemdServiceName(fl validator.FieldLevel) bool {
	serviceName := fl.Field().String()

	if len(serviceName) == 0 || len(serviceName) > 255 {
		return false
	}

	validSuffixes := []string{
		".service",
		".socket",
		".device",
		".mount",
		".automount",
		".swap",
		".target",
		".path",
		".timer",
		".slice",
		".scope",
	}

	hasSuffix := false
	for _, suffix := range validSuffixes {
		if strings.HasSuffix(serviceName, suffix) {
			hasSuffix = true
			break
		}
	}

	if !hasSuffix {
		return false
	}

	return systemdServicePrefixPattern.MatchString(serviceName)
}
