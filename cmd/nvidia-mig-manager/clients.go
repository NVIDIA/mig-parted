package main

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
)

type k8sClients []k8sClient

type k8sClient interface {
	Stop(*kubernetes.Clientset) error
	Restart(*kubernetes.Clientset) error
}

type withNoStopClient struct {
	k8sClient
}

type withNoRestartClient struct {
	k8sClient
}

type pod struct {
	namespace string
	nodename  string
	app       string
}

// An operand is a GPU client that is controlled by a deploy label.
type operand struct {
	pod
	deployLabel string
	lastValue   string
}

func getK8sClients(opts *reconfigureMIGOptions) k8sClients {
	k8sGPUClients := k8sClients{
		&operand{
			pod: pod{
				namespace: opts.GPUClientsNamespace,
				nodename:  opts.NodeName,
				app:       "nvidia-device-plugin-daemonset",
			},
			deployLabel: "nvidia.com/gpu.deploy.device-plugin",
		},
		&operand{
			pod: pod{
				namespace: opts.GPUClientsNamespace,
				nodename:  opts.NodeName,
				app:       "gpu-feature-discovery",
			},
			deployLabel: "nvidia.com/gpu.deploy.gpu-feature-discovery",
		},
		&operand{
			pod: pod{
				namespace: opts.GPUClientsNamespace,
				nodename:  opts.NodeName,
				app:       "nvidia-dcgm-exporter",
			},
			deployLabel: "nvidia.com/gpu.deploy.dcgm-exporter",
		},
		&operand{
			pod: pod{
				namespace: opts.GPUClientsNamespace,
				nodename:  opts.NodeName,
				app:       "nvidia-dcgm",
			},
			deployLabel: "nvidia.com/gpu.deploy.dcgm",
		},
		&operand{
			// TODO: Why don't we wait for the following pd deletion.
			pod: pod{
				namespace: opts.GPUClientsNamespace,
				nodename:  opts.NodeName,
				app:       "",
			},
			deployLabel: "nvidia.com/gpu.deploy.nvsm",
		},
		withNoRestart(&pod{
			namespace: opts.GPUClientsNamespace,
			nodename:  opts.NodeName,
			app:       "nvidia-cuda-validator",
		}),
		withNoRestart(&pod{
			namespace: opts.GPUClientsNamespace,
			nodename:  opts.NodeName,
			app:       "nvidia-device-plugin-validator",
		}),
		withNoStop(&pod{
			namespace: opts.GPUClientsNamespace,
			nodename:  opts.NodeName,
			app:       "nvidia-operator-validator",
		}),
	}

	return k8sGPUClients
}

func (o k8sClients) Restart(clientset *kubernetes.Clientset) error {
	for _, c := range o {
		if c == nil {
			continue
		}
		if err := c.Restart(clientset); err != nil {
			return err
		}
	}
	return nil
}

func (o k8sClients) Stop(clientset *kubernetes.Clientset) error {
	for _, c := range o {
		if c == nil {
			continue
		}
		if err := c.Stop(clientset); err != nil {
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

func (o *withNoRestartClient) Restart(_ *kubernetes.Clientset) error {
	return nil
}

func (o *withNoStopClient) Stop(_ *kubernetes.Clientset) error {
	return nil
}

func (o *pod) Restart(clientset *kubernetes.Clientset) error {
	// TODO: We need to add this namespace to the options.
	err := clientset.CoreV1().Pods(o.namespace).DeleteCollection(
		context.TODO(),
		metav1.DeleteOptions{},
		metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.nodename),
			LabelSelector: fmt.Sprintf("app=%s", o.app),
		},
	)
	if err != nil {
		return fmt.Errorf("unable to delete pods for app %s: %w", o.app, err)
	}
	return nil
}

func (o *pod) Stop(clientset *kubernetes.Clientset) error {
	if o.app == "" {
		return nil
	}
	err := clientset.CoreV1().Pods(o.namespace).DeleteCollection(
		context.TODO(),
		metav1.DeleteOptions{},
		metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.nodename),
			LabelSelector: fmt.Sprintf("app=%s", o.app),
		},
	)
	if err != nil {
		return fmt.Errorf("unable to delete pods for app %s: %w", o.app, err)
	}
	return nil
}

func (o *pod) waitForDeletion(clientset *kubernetes.Clientset) error {
	if o.app == "" {
		return nil
	}
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	watcher, err := clientset.CoreV1().Pods(o.namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.nodename),
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
				pods, err := clientset.CoreV1().Pods(o.namespace).List(ctx, metav1.ListOptions{
					FieldSelector: fmt.Sprintf("spec.nodeName=%s", o.nodename),
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

func (o *operand) Restart(clientset *kubernetes.Clientset) error {
	if o.deployLabel == "" || o.lastValue == "false" {
		return nil
	}
	err := setNodeLabelValue(clientset, o.deployLabel, "true")
	if err != nil {
		return fmt.Errorf("unable to restart operand %q: %w", o.app, err)
	}
	return nil
}

func (o *operand) Stop(clientset *kubernetes.Clientset) error {
	value, err := getNodeLabelValue(clientset, o.deployLabel)
	if err != nil {
		return fmt.Errorf("unable to get the value of the %q label: %w", o.deployLabel, err)
	}
	o.lastValue = value
	// Only set 'paused-*' if the current value is not 'false'.
	// It should only be 'false' if some external entity has forced it to
	// this value, at which point we want to honor it's existing value and
	// not change it.
	if value != "false" {
		if err := setNodeLabelValue(clientset, o.deployLabel, "paused-for-mig-change"); err != nil {
			return err
		}
	}
	return o.waitForDeletion(clientset)
}
