package reconfigure

import (
	"k8s.io/client-go/kubernetes"
)

// An Option represents a functional option passed to the constructor.
type Option func(*options)

// reconfigureMIGOptions contains configuration options for reconfiguring MIG
// settings on a Kubernetes node. This struct is used to manage the various
// parameters required for applying MIG configurations through mig-parted, including node identification, configuration files, reboot behavior, and host
// system service management.
type reconfigureMIGOptions struct {
	// NodeName is the kubernetes node to change the MIG configuration on.
	// Its validation follows the RFC 1123 standard for DNS subdomain names.
	// Source: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
	// NodeName string `validate:"required,hostname_rfc1123"`

	// MIGPartedConfigFile is the mig-parted configuration file path.
	// Deprecated: Pass the config file as an argument.
	MIGPartedConfigFile string `validate:"required,filepath"`

	// SelectedMIGConfig is the selected mig-parted configuration to apply to the
	// node.
	// Deprecated: Pass the selected config as an argument.
	SelectedMIGConfig string

	// DriverLibrayPath is the path to libnvidia-ml.so.1 in the container.
	DriverLibraryPath string `validate:"required,filepath"`

	// WithReboot reboots the node if changing the MIG mode fails for any reason.
	WithReboot bool

	// WithShutdownHostGPUClients shutdowns/restarts any required host GPU clients
	// across a MIG configuration.
	WithShutdownHostGPUClients bool

	// HostRootMount is the container path where host root directory is mounted.
	HostRootMount string `validate:"dirpath"`

	// HostMIGManagerStateFile is the path where the systemd mig-manager state
	// file is located.
	HostMIGManagerStateFile string `validate:"filepath"`

	// HostGPUClientServices is a comma separated list of host systemd services to
	// shutdown/restart across a MIG reconfiguration.
	HostGPUClientServices []string `validate:"dive,systemd_service_name"`

	// HostKubeletService is the name of the host's 'kubelet' systemd service
	// which may need to be shutdown/restarted across a MIG mode reconfiguration.
	HostKubeletService string `validate:"systemd_service_name"`

	configStateLabel string
}

type manager struct {
	clientset *kubernetes.Clientset
	nodeName  string
}

type options struct {
	manager

	driverRoot root

	reconfigureMIGOptions
}

func WithClientset(clientset *kubernetes.Clientset) Option {
	return func(o *options) {
		o.clientset = clientset
	}
}

func WithNodeName(nodeName string) Option {
	return func(o *options) {
		o.nodeName = nodeName
	}
}

func WithDriverRoot[T string | root](driverRoot T) Option {
	return func(o *options) {
		o.driverRoot = root(driverRoot)
	}
}

func WithDriverLibraryPath(driverLibraryPath string) Option {
	return func(o *options) {
		o.DriverLibraryPath = driverLibraryPath
	}
}

func WithShutdownHostGPUClients(shutdownHostGPUClients bool) Option {
	return func(o *options) {
		o.WithShutdownHostGPUClients = shutdownHostGPUClients
	}
}

func WithHostGPUClientServices(hostGPUClientServices ...string) Option {
	return func(o *options) {
		o.HostGPUClientServices = append([]string{}, hostGPUClientServices...)
	}
}

func WithHostKubeletService(hostKubeletService string) Option {
	return func(o *options) {
		o.HostKubeletService = hostKubeletService
	}
}

func WithHostMIGManagerStateFile(hostMIGManagerStateFile string) Option {
	return func(o *options) {
		o.HostMIGManagerStateFile = hostMIGManagerStateFile
	}
}

func WithHostRootMount(hostRootMount string) Option {
	return func(o *options) {
		o.HostRootMount = hostRootMount
	}
}

func WithAllowReboot(allowReboot bool) Option {
	return func(o *options) {
		o.WithReboot = allowReboot
	}
}

func WithConfigStateLabel(configStateLabel string) Option {
	return func(o *options) {
		o.configStateLabel = configStateLabel
	}
}
