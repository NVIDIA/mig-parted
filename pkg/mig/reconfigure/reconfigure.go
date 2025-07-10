package reconfigure

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func New(opts ...Option) (Reconfigurer, error) {
	o := &options{}

	for _, opt := range opts {
		opt(o)
	}

	if o.clientset == nil {
		return nil, fmt.Errorf("a k8s clientset is required")
	}
	if o.nodeName == "" {
		return nil, fmt.Errorf("a node name is required")
	}
	if o.configStateLabel == "" {
		return nil, fmt.Errorf("a config state label must be specified")
	}

	// TODO: Add validation.

	return o, nil
}

func (o *options) Reconfigure(migPartedConfigFile string, selectedMIGConfig string) error {
	// TODO: These should be passed as arguments.
	o.reconfigureMIGOptions.MIGPartedConfigFile = migPartedConfigFile
	o.reconfigureMIGOptions.SelectedMIGConfig = selectedMIGConfig

	return o.reconfigureMIG(&o.reconfigureMIGOptions)
}

func (m *manager) getNodeLabelValue(label string) (string, error) {
	node, err := m.clientset.CoreV1().Nodes().Get(context.TODO(), m.nodeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("unable to get node object: %v", err)
	}

	value, ok := node.Labels[label]
	if !ok {
		return "", nil
	}

	return value, nil
}

func (m *manager) setNodeLabelValue(label, value string) error {
	node, err := m.clientset.CoreV1().Nodes().Get(context.TODO(), m.nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unable to get node object: %v", err)
	}

	labels := node.GetLabels()
	labels[label] = value
	node.SetLabels(labels)
	_, err = m.clientset.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("unable to update node object: %v", err)
	}

	return nil
}
