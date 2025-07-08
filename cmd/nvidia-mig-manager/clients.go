package main

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type k8sClient interface {
	Restart(*kubernetes.Clientset) error
}

// An operand is a GPU client that is controlled by a deploy label.
type operand struct {
	app         string
	deployLabel string
	lastValue   string
}

func stopK8sClients(clientset *kubernetes.Clientset) ([]k8sClient, error) {
	// TODO: We need to add this namespace to the options.
	gpuClientNamespace := "nvidia-gpu-operator"

	var k8sGPUClients []k8sClient

	// We first optionally stop the operands managed by the operator:
	var operands = []*operand{
		{
			app:         "nvidia-device-plugin-daemonset",
			deployLabel: "nvidia.com/gpu.deploy.device-plugin",
		},
		{
			app:         "gpu-feature-discovery",
			deployLabel: "nvidia.com/gpu.deploy.gpu-feature-discovery",
		},
		{
			app:         "nvidia-dcgm-exporter",
			deployLabel: "nvidia.com/gpu.deploy.dcgm-exporter",
		},
		{
			app:         "nvidia-dcgm",
			deployLabel: "nvidia.com/gpu.deploy.dcgm",
		},
		{
			// TODO: Why don't we wait for the following pd deletion.
			app:         "",
			deployLabel: "nvidia.com/gpu.deploy.nvsm",
		},
	}

	pods := []*pod{
		{
			app: "nvidia-cuda-validator",
		},
		{
			app: "nvidia-device-plugin-validator",
		},
	}

	for _, o := range operands {
		value, err := getNodeLabelValue(clientset, o.deployLabel)
		if err != nil {
			return nil, fmt.Errorf("unable to get the value of the %q label: %w", o.deployLabel, err)
		}
		o.lastValue = value
		// Only set 'paused-*' if the current value is not 'false'.
		// It should only be 'false' if some external entity has forced it to
		// this value, at which point we want to honor it's existing value and
		// not change it.
		if value != "false" {
			if err := setNodeLabelValue(clientset, o.deployLabel, "paused-for-mig-change"); err != nil {
				return nil, err
			}
		}
		k8sGPUClients = append(k8sGPUClients, o)
	}

	for _, o := range operands {
		if o.app == "" {
			continue
		}

		// Wait for the nvidia-device-plugin-daemonset pods to be deleted
		timeout := 5 * time.Minute
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		watcher, err := clientset.CoreV1().Pods(gpuClientNamespace).Watch(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeNameFlag),
			LabelSelector: "app=nvidia-device-plugin-daemonset",
		})
		if err != nil {
			return nil, fmt.Errorf("unable to watch pods for deletion: %w", err)
		}
		defer watcher.Stop()

		// Wait for all matching pods to be deleted
		for {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("timeout waiting for pod deletion: %w", ctx.Err())
			case event := <-watcher.ResultChan():
				if event.Type == watch.Deleted {
					// Check if there are any remaining pods matching our criteria
					pods, err := clientset.CoreV1().Pods(gpuClientNamespace).List(ctx, metav1.ListOptions{
						FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeNameFlag),
						LabelSelector: "app=nvidia-device-plugin-daemonset",
					})
					if err != nil {
						return nil, fmt.Errorf("unable to list pods: %w", err)
					}
					if len(pods.Items) == 0 {
						// All pods have been deleted
						break
					}
				}
			}
		}

	}

	for _, p := range pods {
		err := clientset.CoreV1().Pods(gpuClientNamespace).DeleteCollection(
			context.TODO(),
			metav1.DeleteOptions{},
			metav1.ListOptions{
				FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeNameFlag),
				LabelSelector: fmt.Sprintf("app=%s", p.app),
			},
		)
		if err != nil {
			return nil, fmt.Errorf("unable to delete pods for app %s: %w", p.app, err)
		}
	}

	k8sGPUClients = append(k8sGPUClients, &pod{app: "nvidia-operator-validator"})

	return k8sGPUClients, nil
}

func (c *operand) Restart(clientset *kubernetes.Clientset) error {
	if c.deployLabel == "" || c.lastValue == "false" {
		return nil
	}
	err := setNodeLabelValue(clientset, c.deployLabel, "true")
	if err != nil {
		return fmt.Errorf("unable to restart operand %q: %w", c.app, err)
	}
	return nil
}

type pod struct {
	app string
}

func (p *pod) Restart(clientset *kubernetes.Clientset) error {
	// TODO: We need to add this namespace to the options.
	gpuClientNamespace := "nvidia-gpu-operator"
	err := clientset.CoreV1().Pods(gpuClientNamespace).DeleteCollection(
		context.TODO(),
		metav1.DeleteOptions{},
		metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeNameFlag),
			LabelSelector: fmt.Sprintf("app=%s", p.app),
		},
	)
	if err != nil {
		return fmt.Errorf("unable to delete pods for app %s: %w", p.app, err)
	}
	return nil
}
