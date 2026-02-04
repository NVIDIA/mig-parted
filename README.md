# MIG ***Part***iton ***Ed***itor for NVIDIA GPUs

MIG (short for Multi-Instance GPU) is a mode of operation in the newest
generation of NVIDIA Ampere GPUs. It allows one to partition a GPU into a set
of "MIG Devices", each of which appears to the software consuming them as a
mini-GPU with a fixed partition of memory and a fixed partition of compute
resources. Please refer to the [MIG User
Guide](https://docs.nvidia.com/datacenter/tesla/mig-user-guide/index.html) for
a detailed explanation of MIG and the features it provides.

The MIG ***Part***iton ***Ed***itor (`nvidia-mig-parted`) is a tool designed
for system administrators to make working with MIG partitions easier.

It allows administrators to ***declaratively*** define a set of possible MIG
configurations they would like applied to all GPUs on a node. At runtime, they
then point `nvidia-mig-parted` at one of these configurations, and
`nvidia-mig-parted` takes care of applying it. In this way, the same
configuration file can be spread across all nodes in a cluster, and a runtime
flag (or environment variable) can be used to decide which of these
configurations to actually apply to a node at any given time.

As an example, consider the following configuration for an NVIDIA DGX-A100 node
(found in the `examples/config.yaml` file of this repo):
```
version: v1
mig-configs:
  all-disabled:
    - devices: all
      mig-enabled: false

  all-enabled:
    - devices: all
      mig-enabled: true
      mig-devices: {}

  all-1g.5gb:
    - devices: all
      mig-enabled: true
      mig-devices:
        "1g.5gb": 7

  all-2g.10gb:
    - devices: all
      mig-enabled: true
      mig-devices:
        "2g.10gb": 3

  all-3g.20gb:
    - devices: all
      mig-enabled: true
      mig-devices:
        "3g.20gb": 2

  all-balanced:
    - devices: all
      mig-enabled: true
      mig-devices:
        "1g.5gb": 2
        "2g.10gb": 1
        "3g.20gb": 1

  custom-config:
    - devices: [0,1,2,3]
      mig-enabled: false
    - devices: [4]
      mig-enabled: true
      mig-devices:
        "1g.5gb": 7
    - devices: [5]
      mig-enabled: true
      mig-devices:
        "2g.10gb": 3
    - devices: [6]
      mig-enabled: true
      mig-devices:
        "3g.20gb": 2
    - devices: [7]
      mig-enabled: true
      mig-devices:
        "1g.5gb": 2
        "2g.10gb": 1
        "3g.20gb": 1
```
Each of the sections under `mig-configs` is user-defined, with custom labels
used to refer to them. For example, the `all-disabled` label refers to the MIG
configuration that disables MIG for all GPUs on the node. Likewise, the
`all-1g.5gb` label refers to the MIG configuration that slices all GPUs on the
node into `1g.5gb` devices. Finally, the `custom-config` label defines a
completely custom configuration which disables MIG on the first 4 GPUs on the
node, and applies a mix of MIG devices across the rest.

Using this tool the following commands can be run to apply each of these
configs, in turn:
```
$ nvidia-mig-parted apply -f examples/config.yaml -c all-disabled
$ nvidia-mig-parted apply -f examples/config.yaml -c all-1g.5gb
$ nvidia-mig-parted apply -f examples/config.yaml -c all-2g.10gb
$ nvidia-mig-parted apply -f examples/config.yaml -c all-3g.20gb
$ nvidia-mig-parted apply -f examples/config.yaml -c all-balanced
$ nvidia-mig-parted apply -f examples/config.yaml -c custom-config
```

The currently applied configuration can then be looked up with:
```
$ nvidia-mig-parted export
version: v1
mig-configs:
  current:
  - devices: all
    mig-enabled: true
    mig-devices:
      1g.5gb: 2
      2g.10gb: 1
      3g.20gb: 1
```

And asserted with:
```
$ nvidia-mig-parted assert -f examples/config.yaml -c all-balanced
Selected MIG configuration currently applied

$ echo $?
0

$ nvidia-mig-parted assert -f examples/config.yaml -c all-1g.5gb
ERRO[0000] Assertion failure: selected configuration not currently applied

$ echo $?
1
```

**Note:** The `nvidia-mig-parted` tool alone does not take care of making sure
that your node is in a state where MIG mode changes and MIG device
configurations will apply cleanly. Moreover, it does not ensure that MIG device
configurations will persist across node reboots.

To help with this, a `systemd` service and a set of support scripts have been
developed to wrap `nvidia-mig-parted` and provide these much desired features.
Please see the README.md under [deployments/systemd](deployments/systemd) for
more details.

## Installing `nvidia-mig-parted`

At the moment, there is no common distribution platform for
`nvidia-mig-parted`. However, we do build `deb`, `rpm` and `tarball` packages
and distribute them as assets with every release. Please see our release page
[here](https://github.com/NVIDIA/mig-parted/releases) to download them and
install them.

In order to build the program from its source code, you will need to download 
and install the most recent version of the Go programming language. You can 
obtain it from the official website at https://go.dev/. Once you have 
installed Go, please proceed with one of the methods described below.

#### Use `docker` with `go install`:
```
docker run \
    --rm \
    -v $(pwd):/dest \
    golang:1.20.1 \
    sh -c "
    go install github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted@latest
    mv /go/bin/nvidia-mig-parted /dest/nvidia-mig-parted
    "
```

#### Run `go get` and `go install` directly:
```
GO111MODULE=off go get -u github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted
GOBIN=$(pwd)    go install github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted
```

#### Clone the repo and build it:
```
git clone http://github.com/NVIDIA/mig-parted
cd mig-parted
go build ./cmd/nvidia-mig-parted
```

When followed exactly, any of these methods should generate a binary called
`nvidia-mig-parted` in your current directory. Once this is done, it is advised
that you move this binary to somewhere in your path, so you can follow the
commands below verbatim.

## Quick Start

Before going into the details of every possible option for `nvidia-mig-parted`
it's useful to walk through a few examples of its most common usage. All
commands below use the example configuration file found under
`examples/config.yaml` of this repo.

#### Apply a specific MIG config from a configuration file
```
nvidia-mig-parted apply -f examples/config.yaml -c all-1g.5gb
```

#### Apply a config to ***only*** change the MIG mode settings of a config
```
nvidia-mig-parted apply --mode-only -f examples/config.yaml -c all-1g.5gb
```

#### Apply a MIG config with debug output
```
nvidia-mig-parted -d apply -f examples/config.yaml -c all-1g.5gb
```

#### Apply a one-off MIG config without a configuration file
```
cat <<EOF | nvidia-mig-parted apply -f -
version: v1
mig-configs:
  all-1g.5gb:
  - devices: all
    mig-enabled: true
    mig-devices:
      1g.5gb: 7
EOF
```

#### Apply a one-off MIG config to ***only*** change the MIG mode
```
cat <<EOF | nvidia-mig-parted apply --mode-only -f -
version: v1
mig-configs:
  whatever:
  - devices: all
    mig-enabled: true
    mig-devices: {}
EOF
```

#### Export the current MIG config
```
nvidia-mig-parted export
```

#### Assert a specific MIG configuration is currently applied
```
nvidia-mig-parted assert -f examples/config.yaml -c all-1g.5gb
```

#### Assert the MIG mode settings of a MIG configuration are currently applied
```
nvidia-mig-parted assert --mode-only -f examples/config.yaml -c all-1g.5gb
```

#### Assert a one-off MIG config without a configuration file
```
cat <<EOF | nvidia-mig-parted assert -f -
version: v1
mig-configs:
  all-1g.5gb:
  - devices: all
    mig-enabled: true
    mig-devices: 
      1g.5gb: 7
EOF
```

#### Assert the MIG mode setting of a one-off MIG config
```
cat <<EOF | nvidia-mig-parted assert --mode-only -f -
version: v1
mig-configs:
  whatever:
  - devices: all
    mig-enabled: true
    mig-devices: {}
EOF
```

## Known Issues

- `mig-parted` will fail to perform a GPU reset, and therefore toggle the MIG mode on GPUs where a reset is required,
  if the `nvidia_drm` kernel module is loaded. On systems where the `nvidia_drm` kernel module is loaded, one must
  unload it before applying a MIG configuration and load it again after the configuration change has been applied.
  See https://github.com/NVIDIA/mig-parted/issues/181
  