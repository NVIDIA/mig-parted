Name: nvidia-mig-manager
Summary: NVIDIA MIG Partition Editor and Systemd Service
Version: %{version}
Release: %{revision}
Group: Development/Tools
License: Apache Software License 2.0
Vendor: NVIDIA CORPORATION
Packager: NVIDIA CORPORATION <cudatools@nvidia.com>
URL: https://github.com/NVIDIA/mig-parted/deployments/systemd

Source0: LICENSE
Source1: nvidia-mig-parted
Source2: nvidia-mig-manager.service
Source3: nvidia-mig-parted.sh
Source4: override.conf
Source5: service.sh
Source6: utils.sh
Source7: hooks.sh
Source8: hooks-default.yaml
Source9: hooks-minimal.yaml
Source10: config-default.yaml
Source11: nvidia-gpu-reset.target

%description
The NVIDIA MIG Partition Editor allows administrators to declaratively define a
set of possible MIG configurations they would like applied to all GPUs on a
node. At runtime, they then point nvidia-mig-parted at one of these
configurations, and nvidia-mig-parted takes care of applying it. In this way,
the same configuration file can be spread across all nodes in a cluster, and a
runtime flag (or environment variable) can be used to decide which of these
configurations to actually apply to a node at any given time.

The nvidia-mig-manager.service is a systemd service and a set of support hooks
that extend nvidia-mig-parted to provide additional features.

%prep
cp %{SOURCE0} %{SOURCE1} \
   %{SOURCE2} %{SOURCE3} \
   %{SOURCE4} %{SOURCE5} \
   %{SOURCE6} %{SOURCE7} \
   %{SOURCE8} %{SOURCE9} \
   %{SOURCE10} %{SOURCE11} \
    .

%install
mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/usr/lib/systemd/system
mkdir -p %{buildroot}/etc/profile.d
mkdir -p %{buildroot}/etc/systemd/system/nvidia-mig-manager.service.d
mkdir -p %{buildroot}/etc/nvidia-mig-manager
mkdir -p %{buildroot}/var/lib/nvidia-mig-manager

install -m 755 -t %{buildroot}/usr/bin %{SOURCE1}
install -m 644 -t %{buildroot}/usr/lib/systemd/system %{SOURCE2}
install -m 644 -t %{buildroot}/etc/profile.d %{SOURCE3}
install -m 644 -t %{buildroot}/etc/systemd/system/nvidia-mig-manager.service.d %{SOURCE4}
install -m 755 -t %{buildroot}/etc/nvidia-mig-manager %{SOURCE5}
install -m 644 -t %{buildroot}/etc/nvidia-mig-manager %{SOURCE6}
install -m 644 -t %{buildroot}/etc/nvidia-mig-manager %{SOURCE7}
install -m 644 -t %{buildroot}/etc/nvidia-mig-manager %{SOURCE8}
install -m 644 -t %{buildroot}/etc/nvidia-mig-manager %{SOURCE9}
install -m 644 -t %{buildroot}/etc/nvidia-mig-manager %{SOURCE10}
install -m 644 -t %{buildroot}/usr/lib/systemd/system %{SOURCE11}

%files
%license LICENSE
/usr/bin/nvidia-mig-parted
/usr/lib/systemd/system/nvidia-mig-manager.service
%config /etc/profile.d/nvidia-mig-parted.sh
%config /etc/systemd/system/nvidia-mig-manager.service.d/override.conf
/etc/nvidia-mig-manager/service.sh
/etc/nvidia-mig-manager/utils.sh
/etc/nvidia-mig-manager/hooks.sh
/etc/nvidia-mig-manager/config-default.yaml
/etc/nvidia-mig-manager/hooks-default.yaml
/etc/nvidia-mig-manager/hooks-minimal.yaml
%dir /etc/systemd/system/nvidia-mig-manager.service.d
%dir /etc/nvidia-mig-manager/
%dir /var/lib/nvidia-mig-manager
/usr/lib/systemd/system/nvidia-gpu-reset.target

%post
systemctl daemon-reload
systemctl enable nvidia-mig-manager.service

function maybe_add_hooks_symlink() {
  if [ -e /etc/nvidia-mig-manager/hooks.yaml ]; then
    return
  fi

  if ! which nvidia-smi > /dev/null 2>&1; then
    ln -s hooks-default.yaml /etc/nvidia-mig-manager/hooks.yaml
    return
  fi

  local compute_cap=$(nvidia-smi -i 0 --query-gpu=compute_cap --format=csv,noheader)
  if [ "${compute_cap/./}" -ge "90" ] 2> /dev/null; then
    ln -s hooks-minimal.yaml /etc/nvidia-mig-manager/hooks.yaml
  else
    ln -s hooks-default.yaml /etc/nvidia-mig-manager/hooks.yaml
  fi
}

function maybe_add_config_symlink() {
  if [ -e /etc/nvidia-mig-manager/config.yaml ]; then
    return
  fi
  ln -s config-default.yaml /etc/nvidia-mig-manager/config.yaml
}

maybe_add_hooks_symlink
maybe_add_config_symlink

%preun
function maybe_remove_hooks_symlink() {
  local target=$(readlink -f /etc/nvidia-mig-manager/hooks.yaml)
  if [ "${target}" = "/etc/nvidia-mig-manager/hooks-minimal.yaml" ]; then
    rm -rf /etc/nvidia-mig-manager/hooks.yaml
  fi
  if [ "${target}" = "/etc/nvidia-mig-manager/hooks-default.yaml" ]; then
    rm -rf /etc/nvidia-mig-manager/hooks.yaml
  fi
}

function maybe_remove_config_symlink() {
  local target=$(readlink -f /etc/nvidia-mig-manager/config.yaml)
  if [ "${target}" = "/etc/nvidia-mig-manager/config-default.yaml" ]; then
    rm -rf /etc/nvidia-mig-manager/config.yaml
  fi
}

if [ $1 -eq 0 ]
then
  systemctl disable nvidia-mig-manager.service
  systemctl daemon-reload
  maybe_remove_hooks_symlink
  maybe_remove_config_symlink
fi

%changelog
# As of 0.6.0-1 we generate the release information automatically
* %{release_date} NVIDIA CORPORATION <cudatools@nvidia.com> %{version}-%{release}
- See https://github.com/NVIDIA/mig-parted/-/blob/%{git_commit}/CHANGELOG.md
