package main

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// A k8sClient represents a client that can be stoped or restarted.
type k8sClient interface {
	Stop() error
	Restart() error
}

// k8sClients represents a set of clients that can be stopped or restarted
// together.
type k8sClients []k8sClient

var _ k8sClient = (k8sClient)(nil)

// a withNoStopClient wraps the specified client so that the Stop function is a no-op.
type withNoStopClient struct {
	k8sClient
}

// a withNoRestartClient wraps the specified client so that the Restart function is a no-op.
type withNoRestartClient struct {
	k8sClient
}

// A pod represents a kubernetes pod with a specified app= label.
type pod struct {
	app string
	// manager stores a reference to the mig manager managing these clients.
	manager *migManager
}

// An operand is an operand of the GPU Operator that is represented by a pod
// and constolled by a deploy label.
type operand struct {
	*pod
	deployLabel string
	lastValue   string
}

// getK8sClients returns the k8sClients managed by the specified mig manager.
// TODO: This should be configurable so that it can be used as-is from the vgpu-device-manager
// where k8s-clients are not considered.
func (m *migManager) getK8sClients() k8sClients {
	k8sGPUClients := k8sClients{
		m.newOperand("nvidia-device-plugin-daemonset", "nvidia.com/gpu.deploy.device-plugin"),
		m.newOperand("gpu-feature-discovery", "nvidia.com/gpu.deploy.gpu-feature-discovery"),
		m.newOperand("nvidia-dcgm-exporter", "nvidia.com/gpu.deploy.dcgm-exporter"),
		m.newOperand("nvidia-dcgm", "nvidia.com/gpu.deploy.dcgm"),
		// TODO: Why don't we wait for the following pod deletion.
		m.newOperand("", "nvidia.com/gpu.deploy.nvsm"),
		withNoRestart(m.newPod("nvidia-cuda-validator")),
		withNoRestart(m.newPod("nvidia-device-plugin-validator")),
		withNoStop(m.newPod("nvidia-operator-validator")),
	}

	return k8sGPUClients
}

func (m *migManager) newOperand(app string, deployLabel string) *operand {
	return &operand{
		pod:         m.newPod(app),
		deployLabel: deployLabel,
	}
}

func (m *migManager) newPod(app string) *pod {
	return &pod{
		manager: m,
		app:     app,
	}
}

// Restart restarts each of a set of k8s clients.
// The first error encountered is returned and not further clients are restarted.
func (o k8sClients) Restart() error {
	for _, c := range o {
		if c == nil {
			continue
		}
		if err := c.Restart(); err != nil {
			return err
		}
	}
	return nil
}

// Stop stops each of a set of k8s clients.
// The first error encountered is returned and not further clients are stopped.
func (o k8sClients) Stop() error {
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
func withNoRestart(k k8sClient) k8sClient {
	return &withNoRestartClient{k}
}

// withNoStop wraps the specified client so that stopss are disabled.
func withNoStop(k k8sClient) k8sClient {
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
	err := o.manager.clientset.CoreV1().Pods(o.manager.Namespace).DeleteCollection(
		context.TODO(),
		metav1.DeleteOptions{},
		metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.manager.NodeName),
			LabelSelector: fmt.Sprintf("app=%s", o.app),
		},
	)
	if err != nil {
		return fmt.Errorf("unable to delete pods for app %s: %w", o.app, err)
	}
	return nil
}

func (o *pod) waitForDeletion() error {
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	watcher, err := o.manager.clientset.CoreV1().Pods(o.manager.Namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.manager.NodeName),
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
			if event.Type == watch.Deleted {
				// Check if there are any remaining pods matching our criteria
				pods, err := o.manager.clientset.CoreV1().Pods(o.manager.Namespace).List(ctx, metav1.ListOptions{
					FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.manager.NodeName),
					LabelSelector: fmt.Sprintf("app=%s", o.app),
				})
				if err != nil {
					return fmt.Errorf("unable to list pods: %w", err)
				}
				if len(pods.Items) == 0 {
					// All pods have been deleted
					break
				}
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
	err := o.manager.setNodeLabelValue(o.deployLabel, "true")
	if err != nil {
		return fmt.Errorf("unable to restart operand %q: %w", o.app, err)
	}
	return nil
}

// Stop the specified operand by setting its deploy label to 'paused-for-mig-change'.
// If the deploy label is already set to false, this is assumed to be controlled
// by an external entity and no changes are made.
func (o *operand) Stop() error {
	value, err := o.manager.getNodeLabelValue(o.deployLabel)
	if err != nil {
		return fmt.Errorf("unable to get the value of the %q label: %w", o.deployLabel, err)
	}
	o.lastValue = value
	if value != "false" {
		if err := o.manager.setNodeLabelValue(o.deployLabel, "paused-for-mig-change"); err != nil {
			return err
		}
	}
	// TODO: For the nvidia.com/gpu.deploy.nvsm label we have no associated app name.
	if o.app == "" {
		return nil
	}
	return o.waitForDeletion()
}
