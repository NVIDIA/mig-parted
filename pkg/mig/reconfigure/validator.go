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

package reconfigure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// RestoreValidator sets the operator-validator deploy label back to "true" if a
// MIG change left it paused. It must not run concurrently with a reconfiguration.
func RestoreValidator(ctx context.Context, client kubernetes.Interface, nodeName string) error {
	err := wait.ExponentialBackoffWithContext(ctx, wait.Backoff{Duration: 10 * time.Millisecond, Factor: 5, Steps: 4}, func(ctx context.Context) (bool, error) {
		node, err := client.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if node.Labels[operatorValidatorDeployLabel] != validatorPaused {
			return true, nil
		}
		patch, err := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{
				"resourceVersion": node.ResourceVersion,
				"labels":          map[string]string{operatorValidatorDeployLabel: "true"},
			},
		})
		if err != nil {
			return false, err
		}
		_, err = client.CoreV1().Nodes().Patch(ctx, nodeName, types.MergePatchType, patch, metav1.PatchOptions{})
		if apierrors.IsConflict(err) {
			return false, nil
		}
		return err == nil, err
	})
	if err != nil {
		return fmt.Errorf("failed to restore operator-validator on node %s: %w", nodeName, err)
	}
	return nil
}
