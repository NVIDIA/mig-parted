package main

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type k8sClients []k8sClient

type k8sClient interface {
	Stop() error
	Restart() error
}

type withNoStopClient struct {
	k8sClient
}

type withNoRestartClient struct {
	k8sClient
}

type pod struct {
	app string
	// manager stores a reference to the mig manager managing these clients.
	manager *migManager
}

// An operand is a GPU client that is controlled by a deploy label.
type operand struct {
	pod
	deployLabel string
	lastValue   string
}

func (m *migManager) getK8sClients(opts *reconfigureMIGOptions) k8sClients {
	k8sGPUClients := k8sClients{
		&operand{
			pod: pod{
				app:     "nvidia-device-plugin-daemonset",
				manager: m,
			},
			deployLabel: "nvidia.com/gpu.deploy.device-plugin",
		},
		&operand{
			pod: pod{
				app:     "gpu-feature-discovery",
				manager: m,
			},
			deployLabel: "nvidia.com/gpu.deploy.gpu-feature-discovery",
		},
		&operand{
			pod: pod{
				app:     "nvidia-dcgm-exporter",
				manager: m,
			},
			deployLabel: "nvidia.com/gpu.deploy.dcgm-exporter",
		},
		&operand{
			pod: pod{
				app:     "nvidia-dcgm",
				manager: m,
			},
			deployLabel: "nvidia.com/gpu.deploy.dcgm",
		},
		&operand{
			// TODO: Why don't we wait for the following pd deletion.
			pod: pod{
				app:     "",
				manager: m,
			},
			deployLabel: "nvidia.com/gpu.deploy.nvsm",
		},
		withNoRestart(&pod{
			app:     "nvidia-cuda-validator",
			manager: m,
		}),
		withNoRestart(&pod{
			app:     "nvidia-device-plugin-validator",
			manager: m,
		}),
		withNoStop(&pod{
			app:     "nvidia-operator-validator",
			manager: m,
		}),
	}

	return k8sGPUClients
}

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

func withNoRestart(k k8sClient) k8sClient {
	return &withNoRestartClient{k}
}

func withNoStop(k k8sClient) k8sClient {
	return &withNoStopClient{k}
}

func (o *withNoRestartClient) Restart() error {
	return nil
}

func (o *withNoStopClient) Stop() error {
	return nil
}

func (o *pod) Restart() error {
	// TODO: We need to add this namespace to the options.
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

func (o *pod) Stop() error {
	if o.app == "" {
		return nil
	}
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
	if o.app == "" {
		return nil
	}
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

func (o *operand) Restart() error {
	if o.deployLabel == "" || o.lastValue == "false" {
		return nil
	}
	err := o.manager.setNodeLabelValue(o.deployLabel, "true")
	if err != nil {
		return fmt.Errorf("unable to restart operand %q: %w", o.app, err)
	}
	return nil
}

func (o *operand) Stop() error {
	value, err := o.manager.getNodeLabelValue(o.deployLabel)
	if err != nil {
		return fmt.Errorf("unable to get the value of the %q label: %w", o.deployLabel, err)
	}
	o.lastValue = value
	// Only set 'paused-*' if the current value is not 'false'.
	// It should only be 'false' if some external entity has forced it to
	// this value, at which point we want to honor it's existing value and
	// not change it.
	if value != "false" {
		if err := o.manager.setNodeLabelValue(o.deployLabel, "paused-for-mig-change"); err != nil {
			return err
		}
	}
	return o.waitForDeletion()
}
