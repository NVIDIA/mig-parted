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
