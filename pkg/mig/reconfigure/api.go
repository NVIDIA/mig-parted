package reconfigure

import "os/exec"

const (
	MIGConfigStateLabel  = "nvidia.com/mig.config.state"
	VGPUConfigStateLabel = "nvidia.com/vgpu.config.state"
)

// A Reconfigurer applies a specified MIG configuration.
type Reconfigurer interface {
	Reconfigure() error
}

// A commandRunner is used to run a constructed command.
// This interface allows us to inject a runner for testing.
//
//go:generate moq -rm -fmt=goimports -out command-runner_mock.go . commandRunner
type commandRunner interface {
	Run(*exec.Cmd) error
}

// migParted defines an interface for interacting with mig-parted.
//
//go:generate moq -rm -fmt=goimports -out mig-parted_mock.go . migParted
type migParted interface {
	assertValidMIGConfig() error
	assertMIGConfig() error
	assertMIGModeOnly() error
	applyMIGModeOnly() error
	applyMIGConfig() error
}

// nodeLabeller defines an interface for interacting with node labels.
//
//go:generate moq -rm -fmt=goimports -out node-labeller_mock.go . nodeLabeller
type nodeLabeller interface {
	getNodeLabelValue(string) (string, error)
	setNodeLabelValue(string, string) error
	getK8sGPUClients(string) gpuClients
}
