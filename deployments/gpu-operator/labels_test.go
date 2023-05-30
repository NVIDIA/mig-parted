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

import "testing"

func TestMaybeSetTrue(t *testing.T) {
	//Test 1: value is "false"
	result := maybeSetTrue("false")
	if result != "false" {
		t.Errorf("Expected 'false', got '%s'", result)
	}

	//Test 2: value is "true"
	result = maybeSetTrue("true")
	if result != "true" {
		t.Errorf("Expected 'true', got '%s'", result)
	}
}

func TestMaybeSetPaused(t *testing.T) {
	//Test 1: value is "false"
	result := maybeSetPaused("false")
	if result != "false" {
		t.Errorf("Expected 'false', got '%s'", result)
	}

	//Test 2: value is "true"
	result = maybeSetPaused("true")
	if result != "paused-for-mig-change" {
		t.Errorf("Expected 'paused-for-mig-change', got '%s'", result)
	}
}

