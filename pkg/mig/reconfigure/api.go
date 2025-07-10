package reconfigure

const (
	MIGConfigStateLabel  = "nvidia.com/mig.config.state"
	VGPUConfigStateLabel = "nvidia.com/vgpu.config.state"
)

// A Reconfigurer applies a specified MIG configuration.
type Reconfigurer interface {
	Reconfigure() error
}
