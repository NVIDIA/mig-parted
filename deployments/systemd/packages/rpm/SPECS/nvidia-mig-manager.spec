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
Source10: config.yaml

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
   %{SOURCE10} \
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

%files
%license LICENSE
/usr/bin/nvidia-mig-parted
/usr/lib/systemd/system/nvidia-mig-manager.service
%config /etc/profile.d/nvidia-mig-parted.sh
%config /etc/systemd/system/nvidia-mig-manager.service.d/override.conf
/etc/nvidia-mig-manager/service.sh
/etc/nvidia-mig-manager/utils.sh
/etc/nvidia-mig-manager/hooks.sh
%config /etc/nvidia-mig-manager/config.yaml
/etc/nvidia-mig-manager/hooks-default.yaml
/etc/nvidia-mig-manager/hooks-minimal.yaml
%dir /etc/systemd/system/nvidia-mig-manager.service.d
%dir /etc/nvidia-mig-manager/
%dir /var/lib/nvidia-mig-manager

%post
systemctl daemon-reload
systemctl enable nvidia-mig-manager.service

function maybe_add_hooks_symlink() {
  if [ -e /etc/nvidia-mig-manager/hooks.yaml ]; then
    return
  fi

  which nvidia-smi > /dev/null 2>&1
  if [ "${?}" != 0 ]; then
    return
  fi

  local compute_cap=$(nvidia-smi -i 0 --query-gpu=compute_cap --format=csv,noheader)
  if [ "${compute_cap/./}" -ge "90" ]; then
    ln -s hooks-minimal.yaml /etc/nvidia-mig-manager/hooks.yaml
  else
    ln -s hooks-default.yaml /etc/nvidia-mig-manager/hooks.yaml
  fi
}

maybe_add_hooks_symlink

%preun
systemctl disable nvidia-mig-manager.service
systemctl daemon-reload

function maybe_remove_hooks_symlink() {
  local target=$(readlink -f /etc/nvidia-mig-manager/hooks.yaml)
  if [ "${target}" = "/etc/nvidia-mig-manager/hooks-minimal.yaml" ]; then
    rm -rf /etc/nvidia-mig-manager/hooks.yaml
  fi
  if [ "${target}" = "/etc/nvidia-mig-manager/hooks-default.yaml" ]; then
    rm -rf /etc/nvidia-mig-manager/hooks.yaml
  fi
}

maybe_remove_hooks_symlink

%changelog
* Thu Jun 16 2022 NVIDIA CORPORATION <cudatools@nvidia.com> 0.4.2-1
- Update CUDA image to 11.7.0
- Add extra assert in k8s-mig-manager to double check mig-mode change applied
- Update mig-manager image to use NGC DL license

* Mon May 30 2022 NVIDIA CORPORATION <cudatools@nvidia.com> 0.4.1-1
- Keep NVML alive across all mig-parted commands (except GPU reset)
- Remove unnecessary services from hooks.sh

* Tue Apr 05 2022 NVIDIA CORPORATION <cudatools@nvidia.com> 0.4.0-1
- Update nvidia-mig-parted.sh to include MIG_PARTED_CHECKPOINT_FILE
- Add checkpoint / restore commands to mig-parted CLI
- Update golang version to 1.16.4
- Support instantiation of *_PROFILE_6_SLICE GIs and CIs
- Update cyrus-sasl-lib to address CVE-2022-24407
- Add support for MIG profiles with +me as an attribute extension
- Support Compute Instances in mig-parted config such that CI != GI
- Update go-nvml to v0.11.6
- Change semantics of 'all' to mean 'all-mig-capable' in mig-parted config

* Fri Mar 18 2022 NVIDIA CORPORATION <cudatools@nvidia.com> 0.3.0-1
- k8s-mig-manager: Add support for multi-arch images
- k8s-mig-manager: Handle eviction of NVSM pod when applying MIG changes

* Wed Nov 17 2021 NVIDIA CORPORATION <cudatools@nvidia.com> 0.2.0-1
- nvidia-mig-parted:   Support passing newer GI and CI profile enums on older drivers
- k8s-mig-manager:     Rename nvcr.io/nvidia to nvcr.io/nvidia/cloud-native
- k8s-mig-manager:     Add support for pre-installed drivers
- systemd-mig-manager: Update logic to remove 'containerd' containers in utils.sh
- systemd-mig-manager: Update logic to shutdown only active systemd services in list
- ci-infrastructure:   Rework build and CI to align with other projects
- ci-infrastructure:   Use pulse instead of contamer for scans

* Mon Sep 20 2021 NVIDIA CORPORATION <cudatools@nvidia.com> 0.1.3-1
- Add default configs for the PG506-96GB card
- Remove CombinedMigManager and add wrappers for Mode/Config Managers
- Add a function to check the minimum NVML version required
- Add SystemGetNVMLVersion() to the NVML interface
- Fix small bug in assert logic for non MIG-capable GPUs

* Thu Aug 05 2021 NVIDIA CORPORATION <cudatools@nvidia.com> 0.1.2-1
- Do not start nvidia-mig-manager.service when installing the .deb
- Restore lost assert_gpu_reset_available() function
- Add nvidia-dcgm.service to driver_services array
- Split dcgm, and dcgm-exporter in k8s-mig-manager

* Wed May 19 2021 NVIDIA CORPORATION <cudatools@nvidia.com> 0.1.1-1
- Update packaged config.yaml to include more supported devices

* Fri May 07 2021 NVIDIA CORPORATION <cudatools@nvidia.com> 0.1.0-1
- Initial release of rpm package for v0.1.0
