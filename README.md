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

The currently applied configuration can then be exported with:
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

**Note:** The `assert` feature is most useful in combination with the
`--mode-only` flag (described in [this
section](#switching-mig-mode-without-the-nvidia-driver-loaded) below) to verify
that MIG mode is already set properly on all GPUs and skip rebooting the node
when running under GPU passthrough virtualization. On first boot the assertion
will fail, and the node will be configured and rebooted. After reboot, the
assertion will succeed and the reconfiguration / reboot is skipped.

```
#!/usr/bin/env bash 

nvidia-mig-parted assert --mode-only -f examples/config.yaml -c all-enabled
if [ "$?" != "0" ]; then 
	nvidia-mig-parted apply --mode-only -f examples/config.yaml -c all-enabled
	reboot
fi
```

## Installing `nvidia-mig-parted`

At the moment, there is no common distribution platform for
`nvidia-mig-parted`, and the only way to get it is to build it from source.
There are two common methods.

#### Run `go install`:
```
go get github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted
GOBIN=$(pwd) go install github.com/NVIDIA/mig-parted/cmd/nvidia-mig-parted
```

#### Clone the repo and build it:
```
git clone http://github.com/NVIDIA/mig-parted
cd mig-parted
go build ./cmd/nvidia-mig-parted
```

When followed exactly, both of these methods should generate a binary called
`nvidia-mig-parted` in your current directory. Once this is done, it is advised
that you move this binary to somewhere in your path so you can follow the
commands below verbatim.

## Quick Start

Before going into the details of every possible option for `nvidia-mig-parted`
its useful to walk through a few examples of its most common usage. All
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

## Switching MIG mode **without** the NVIDIA driver loaded
One important feature of `nvidia-mig-parted` is that it can toggle the MIG mode
of a set of GPUs independent of partitioning them into a set of MIG devices.

For example, this can be done via calls to:
```
$ nvidia-mig-parted apply --mode-only -f examples/config.yaml -c all-disabled
$ nvidia-mig-parted apply --mode-only -f examples/config.yaml -c all-enabled
$ nvidia-mig-parted apply --mode-only -f examples/config.yaml -c custom-config
...
```

Under the hood, `nvidia-mig-parted` will scan the selected configuration and
only apply the `mig-enabled` directive for each GPU (skipping configuration of
the MIG devices specified).

A subsequent call without the `--mode-only` flag is then needed to
trigger the MIG devices to actually get created.

Moreover, it is able to perform the MIG mode switch on a GPU ***without*** having
the NVIDIA GPU driver loaded. This is important for several reasons, outlined below.

As described in the [MIG user
guide](https://docs.nvidia.com/datacenter/tesla/mig-user-guide/index.html#enable-mig-mode),
enabling and disabling MIG mode is a heavy weight operation that (in all cases)
requires a GPU reset and (in some cases) requires a full node reboot (i.e. when
running in a VM with GPU passthrough). Moreover, it is not always possible to
perform a GPU reset (even if it is technically allowed) because the NVIDIA
driver will block the reset if there are any clients currently attached to the
GPU. Depending on what software is installed along side the NVIDIA driver,
enumerating these clients, disconnecting them, and reattaching them is often
cumbersome (if not impossible) to do correctly.

Having the ability to change MIG mode without the driver installed, gives
administrator's the flexibility to perform this switch without making any
assumptions about the rest of the software stack running on the node.

For example, the MIG mode switch can be done very early on in the node boot
process, before any other software comes online. Once the rest of the stack
comes up, subsequent calls to `nvidia-mig-parted ` can be made made that change
the set of MIG devices that are present, but no longer require a MIG mode
switch or a GPU reset.

In other scenarios (most notably in the cloud), it may be desirable to
"pre-enable" MIG for GPUs attached to nodes that have no GPU driver installed
in their base OS. In such a scenario, something like Amazon's `cloud-config` or
GCP's `cloud-init` script can be used to ensure MIG mode is configured as
desired (and perform a node reboot if necessary). Once the node is up, the user
can then install the nvidia-driver themselves and proceed with configuring any
MIG devices.
