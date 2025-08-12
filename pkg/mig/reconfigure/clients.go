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
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/klog/v2"
)

// A gpuClient represents a client that can be stoped or restarted.
type gpuClient interface {
	Stop() error
	Restart() error
}

// gpuClients represents a set of clients that can be stopped or restarted
// together.
type gpuClients []gpuClient

var _ gpuClient = (gpuClient)(nil)

// a withNoStopClient wraps the specified client so that the Stop function is a no-op.
type withNoStopClient struct {
	gpuClient
}

// a withNoRestartClient wraps the specified client so that the Restart function is a no-op.
type withNoRestartClient struct {
	gpuClient
}

// A pod represents a kubernetes pod with a specified app= label.
type pod struct {
	namespace string
	app       string
	node      *node
}

// An operand is an operand of the GPU Operator that is represented by a pod
// and constolled by a deploy label.
type operand struct {
	*pod
	deployLabel string
	lastValue   string
}

// A systemdService is a GPU client running as a systemd service.
type systemdService struct {
	name          string
	hostRootMount string
}

// getK8sGPUClients returns the gpuClients on the specified node.
// TODO: This should be configurable so that it can be used as-is from the vgpu-device-manager
// where k8s-clients are not considered.
func (n *node) getK8sGPUClients(namespace string) gpuClients {
	if namespace == "" {
		return nil
	}

	return gpuClients{
		n.newOperand(namespace, "nvidia-device-plugin-daemonset", "nvidia.com/gpu.deploy.device-plugin"),
		n.newOperand(namespace, "gpu-feature-discovery", "nvidia.com/gpu.deploy.gpu-feature-discovery"),
		n.newOperand(namespace, "nvidia-dcgm-exporter", "nvidia.com/gpu.deploy.dcgm-exporter"),
		n.newOperand(namespace, "nvidia-dcgm", "nvidia.com/gpu.deploy.dcgm"),
		// TODO: Why don't we wait for the following pod deletion.
		n.newOperand(namespace, "", "nvidia.com/gpu.deploy.nvsm"),
		withNoRestart(n.newPod(namespace, "nvidia-cuda-validator")),
		withNoRestart(n.newPod(namespace, "nvidia-device-plugin-validator")),
	}
}

func (n *node) newOperand(namespace string, app string, deployLabel string) *operand {
	return &operand{
		pod:         n.newPod(namespace, app),
		deployLabel: deployLabel,
	}
}

func (n *node) newPod(namespace string, app string) *pod {
	return &pod{
		node:      n,
		namespace: namespace,
		app:       app,
	}
}

// Restart restarts each of a set of k8s clients.
// The first error encountered is returned and not further clients are restarted.
func (o gpuClients) Restart() error {
	var errs error
	for i := range len(o) {
		c := o[len(o)-i-1]
		if err := c.Restart(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

// Stop stops each of a set of k8s clients.
// The first error encountered is returned and not further clients are stopped.
func (o gpuClients) Stop() error {
	for _, c := range o {
		if c == nil {
			continue
		}
		if err := c.Stop(); err != nil {
			return err
		}
	}
	return nil
}

// withNoRestart wraps the specified client so that restarts are disabled.
func withNoRestart(k gpuClient) gpuClient {
	return &withNoRestartClient{k}
}

// withNoStop wraps the specified client so that stopss are disabled.
func withNoStop(k gpuClient) gpuClient {
	return &withNoStopClient{k}
}

func (o *withNoRestartClient) Restart() error {
	return nil
}

func (o *withNoStopClient) Stop() error {
	return nil
}

// Restart the specified pod.
func (o *pod) Restart() error {
	return o.delete()
}

// Stop the specified pod.
func (o *pod) Stop() error {
	return o.delete()
}

func (o *pod) delete() error {
	klog.InfoS("Deleting pod", "app", o.app, "node", o.node.name, "namespace", o.namespace)
	args := []string{
		"delete", "pod",
		"--field-selector \"spec.nodeName=" + o.node.name + "\"",
		"-n \"" + o.namespace + "\"",
		"-l app=" + o.app,
	}

	klog.InfoS("TODO: running kubctl with args", "args", args)
	// TODO: We should use the klientset here, but we don't have the correct
	// permissions.
	cmd := exec.Command("kubectl", args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	err := cmd.Run()
	// err := o.node.clientset.CoreV1().Pods(o.namespace).DeleteCollection(
	// 	context.TODO(),
	// 	metav1.DeleteOptions{},
	// 	metav1.ListOptions{
	// 		FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.node.name),
	// 		LabelSelector: fmt.Sprintf("app=%s", o.app),
	// 	},
	// )
	if err != nil {
		return fmt.Errorf("unable to delete pods for app %s: %w", o.app, err)
	}
	return nil
}

func (o *pod) waitForDeletion() error {
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pods, err := o.node.clientset.CoreV1().Pods(o.namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.node.name),
		LabelSelector: fmt.Sprintf("app=%s", o.app),
	})
	if err != nil {
		return fmt.Errorf("unable to list pods for %v: %w", o.app, err)
	}
	if len(pods.Items) == 0 {
		klog.InfoS("No pods found; skipping", "app", o.app, "node", o.node.name, "namespace", o.namespace)
		return nil
	}

	klog.InfoS("Waiting for shutdown", "app", o.app, "node", o.node.name, "namespace", o.namespace)
	watcher, err := o.node.clientset.CoreV1().Pods(o.namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.node.name),
		LabelSelector: fmt.Sprintf("app=%s", o.app),
	})
	if err != nil {
		return fmt.Errorf("unable to watch pods for deletion: %w", err)
	}
	defer watcher.Stop()

	// Wait for all matching pods to be deleted
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for pod deletion: %w", ctx.Err())
		case event := <-watcher.ResultChan():
			klog.InfoS("got watcher result", "event.Type", event.Type)
			if event.Type == watch.Deleted {
				// Check if there are any remaining pods matching our criteria
				pods, err := o.node.clientset.CoreV1().Pods(o.namespace).List(ctx, metav1.ListOptions{
					FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.node.name),
					LabelSelector: fmt.Sprintf("app=%s", o.app),
				})
				if err != nil {
					return fmt.Errorf("unable to list pods: %w", err)
				}
				if len(pods.Items) == 0 {
					klog.InfoS("Shutdown", "app", o.app, "node", o.node.name, "namespace", o.namespace)
					// All pods have been deleted
					return nil
				}
				klog.InfoS("Still waiting for pods", "pods", pods)
			}
		}
	}
}

// Restart the specified operand by setting its deployLabel to 'true'
// If the deploy label is already set to false, this is assumed to be controlled
// by an external entity and no changes are made.
func (o *operand) Restart() error {
	if o.lastValue == "false" {
		return nil
	}
	klog.InfoS("Restarting operand", "app", o.app, "node", o.node.name, "namespace", o.namespace)

	err := o.node.setNodeLabelValue(o.deployLabel, "true")
	if err != nil {
		return fmt.Errorf("unable to restart operand %q: %w", o.app, err)
	}
	return nil
}

// Stop the specified operand by setting its deploy label to 'paused-for-mig-change'.
// If the deploy label is already set to false, this is assumed to be controlled
// by an external entity and no changes are made.
func (o *operand) Stop() error {
	value, err := o.node.getNodeLabelValue(o.deployLabel)
	if err != nil {
		return fmt.Errorf("unable to get the value of the %q label: %w", o.deployLabel, err)
	}
	o.lastValue = value
	if value != "false" {
		klog.InfoS("Stopping operand", "app", o.app, "node", o.node.name, "namespace", o.namespace)
		if err := o.node.setNodeLabelValue(o.deployLabel, "paused-for-mig-change"); err != nil {
			return err
		}
	}
	// TODO: For the nvidia.com/gpu.deploy.nvsm label we have no associated app name.
	if o.app == "" {
		return nil
	}
	return o.waitForDeletion()
}

func (opts *reconfigureMIGOptions) newSystemdService(name string) *systemdService {
	return &systemdService{
		name:          name,
		hostRootMount: opts.HostRootMount,
	}
}

func (o *systemdService) Restart() error {
	cmd := exec.Command("chroot", o.hostRootMount, "systemctl", "start", o.name) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	return cmd.Run()
}

func (o *systemdService) Stop() error {
	cmd := exec.Command("chroot", o.hostRootMount, "systemctl", "stop", o.name) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name, action is controlled parameter.
	return cmd.Run()
}

// Pause stops the specified systemd service and returns whether it should be restarted.
func (o *systemdService) Pause() (bool, error) {
	if err := o.assertIsActive(); err == nil {
		log.Infof("pause %s (active, will-restart)", o.name)
		if err := o.Stop(); err != nil {
			// If we fail to stop a server, we don't restart the service.
			return false, err
		}
		// An active service that was successfully stopped must be restarted.
		return true, nil
	}

	return o.shouldRestart()
}

func (o *systemdService) shouldRestart() (bool, error) {
	if err := o.assertIsActive(); err != nil {
		return false, nil
	}

	// TODO: Is there a better way to check existence?
	// In the shell script we check the output to be empty.
	if err := o.assertIsEnabled(); err != nil {
		log.Infof("Skipping %s (no-exist)", o.name)
		return false, nil
	}

	if err := o.assertIsFailed(); err == nil {
		log.Infof("Skipping %s (is-failed, will-restart)", o.name)
		return true, nil
	}

	if err := o.assertIsEnabled(); err != nil {
		log.Infof("Skipping %s (disabled)", o.name)
		return false, nil
	}

	serviceType, err := o.getType()
	if err != nil {
		return false, err
	}
	if serviceType == "oneshot" {
		log.Infof("Skipping %s (inactive, oneshot, no-restart)", o.name)
		return false, nil
	}

	log.Infof("Skipping %s (type=%s, inactive, will-restart)", o.name, serviceType)
	return true, nil
}

func (o *systemdService) assertIsActive() error {
	cmd := exec.Command("chroot", o.hostRootMount, "systemctl", "-q", "is-active", o.name) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	return cmd.Run()
}

func (o *systemdService) assertIsEnabled() error {
	cmd := exec.Command("chroot", o.hostRootMount, "systemctl", "-q", "is-enabled", o.name) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	return cmd.Run()
}

func (o *systemdService) assertIsFailed() error {
	cmd := exec.Command("chroot", o.hostRootMount, "systemctl", "-q", "is-failed", o.name) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	return cmd.Run()
}

func (o *systemdService) getType() (string, error) {
	cmd := exec.Command("chroot", o.hostRootMount, "systemctl", "show", "--property=Type", o.name) // #nosec G204 -- HostRootMount validated via dirpath, service validated via systemd_service_name.
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(output)), "Type="), nil
}
