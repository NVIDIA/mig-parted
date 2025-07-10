package reconfigure

const (
	MIGConfigStateLabel  = "nvidia.com/mig.config.state"
	VGPUConfigStateLabel = "nvidia.com/vgpu.config.state"
)

// A Reconfigurer applies applies applies the specified config.
type Reconfigurer interface {
	Reconfigure() error
}
