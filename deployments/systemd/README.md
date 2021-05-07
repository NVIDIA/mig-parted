# The `nvidia-mig-manager.service`

The `nvidia-mig-manager.service` is a `systemd` service and a set of support
hooks that extend `nvidia-mig-parted` to provide the following features:

1. When applying a MIG mode change, shutdown (and restart) a *user-specific*
   set of NVIDIA driver clients so that a GPU reset can be applied to persist
   the MIG mode setting. A list of common clients is provided by default, but
   users can customize this list to their own particular needs. Please see the
   provided `hooks.sh` and `hooks.yaml` files for details on what is run and
   how to change it.

1. When applying a MIG device configuration, shutdown (and restart) a *user-specific*
   set of services so that they will pick up any MIG device changes once they
   come back online. Common services such as the `k8s-device-plugin`,
   `gpu-feature-discovery` and `dcgm-exporter` are provided by default, but
   this list can be customized as needed. Please see the provided `hooks.sh`
   and `hooks.yaml` files for details on what is run and how to change it.

1. Persist MIG device configurations across node reboots. So long as the
   provided `hooks.yaml` file is used when performing any desired MIG
   configurations, those changes will be reapplied every time the node reboots.

1. Apply MIG mode changes ***without*** requiring the NVIDIA driver to be
   installed or loaded. Situations where this is important are outlined in more detail
   [below](#switching-mig-mode-without-the-nvidia-driver-loaded).

1. When a node is first coming online, automatically reboot it (if required) to
   force MIG mode changes to take effect when GPU reset is unavailable (i.e.
   under GPU Passthrough virtualization). A failsafe is in place to ensure
   that such reboots only happen *once* without manual interventions if things
   go wrong.

To install the `nvidia-mig-manager.service` simply run `./install.sh` from the
directory where this README is located.

**Note:** At the moment, `docker` is a prerequisite to this installation
because it runs a container with `go` in it to download and build the latest
`nvidia-mig-parted` before installing it. We plan to relax this requirement in
the near future.

The following files will be added as part of this installation: 

* `/usr/bin/nvidia-mig-parted`
* `/usr/lib/systemd/system/nvidia-mig-manager.service`
* `/etc/systemd/system/nvidia-mig-manager.service.d/override.conf`
* `/etc/profile.d/nvidia-mig-parted.sh`
* `/etc/nvidia-mig-manager/utils.sh`
* `/etc/nvidia-mig-manager/service.sh`
* `/etc/nvidia-mig-manager/hooks.sh`
* `/etc/nvidia-mig-manager/hooks.yaml`
* `/etc/nvidia-mig-manager/config.yaml`

Users should only need to customize the `config.yaml` (to add any user-specific
MIG configurations they would like to apply) and the `hooks.sh` and
`hooks.yaml` files (to add any user specific services that need to be shutdown
and restarted when applying a MIG configuration).

Once installed, new MIG configurations can be applied at any time by running
`nvidia-mig-parted apply` and pointing it at the `config.yaml` and `hooks.yaml`
files in `/etc/nvidia-mig-manager`.

By default, environment variables for the config and hooks files are set in
`/etc/profile.d/mig-parted.sh` as part of this installation. Meaning that
once this service is installed, anytime you login, these variables should be
set for you:
```
export MIG_PARTED_CONFIG_FILE=/etc/nvidia-mig-manager/config.yaml
export MIG_PARTED_HOOKS_FILE=/etc/nvidia-mig-manager/hooks.yaml
```

As noted above, these hooks do everything they can to ensure that the services
are started and stopped so that the new configuration is applied cleanly. If
for some reason the config just won't seem to apply (because the full set of
services that need to be stopped / started are too difficult to enumerate), the
node can always be rebooted (as a *very* last resort), at which point the
config should now be in place.

Below are some examples of how one might run this in a production setting:
```
# Using ansible
- name: Apply a new MIG configuration 
  environment:
      MIG_PARTED_DEBUG: false
      MIG_PARTED_HOOKS_FILE: /etc/nvidia-mig-manager/hooks.yaml
      MIG_PARTED_CONFIG_FILE: /etc/nvidia-mig-manager/config.yaml
      MIG_PARTED_SELECTED_CONFIG: all-1g.5gb
  command: /usr/bin/nvidia-mig-parted apply

# Using docker
docker run \
    --rm \
    --privileged \
    --ipc=host \
    --pid=host \
    -v /:/host \
    -e MIG_PARTED_DEBUG=false \
    -e MIG_PARTED_HOOKS_FILE=/etc/nvidia-mig-manager/hooks.yaml \
    -e MIG_PARTED_CONFIG_FILE=/etc/nvidia-mig-manager/config.yaml \
    -e MIG_PARTED_SELECTED_CONFIG=all-1g.5gb \
    library/alpine \
    sh -c "exec chroot /host /usr/bin/nvidia-mig-parted apply"

# Using kubernetes
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: nvidia-mig-parted
spec:
  restartPolicy: Never
  hostPID: true
  hostIPC: true
  containers:
  - name: nvidia-mig-parted
    image: library/alpine
    imagePullPolicy: IfNotPresent
    env:
    - name: MIG_PARTED_DEBUG
      value: "false"
    - name: MIG_PARTED_HOOKS_FILE
      value: "/etc/nvidia-mig-manager/hooks.yaml"
    - name: MIG_PARTED_CONFIG_FILE
      value: "/etc/nvidia-mig-manager/config.yaml"
    - name: MIG_PARTED_SELECTED_CONFIG
      value: "all-1g.5gb"
    command: ["sh", "-c", "exec chroot /host nvidia-mig-parted apply"]
    securityContext:
      privileged: true
    volumeMounts:
    - mountPath: /host
      name: host-root
  volumes:
  - name: host-root
    hostPath:
      path: "/"
      type: Directory
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
