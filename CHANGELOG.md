# NVIDIA MIG Manager Changelog

## v0.13.0
- Fix device-filter for all-1g.24gb mig-parted config
- Port reconfigure-mig.sh to Go
- Move to distroless as the base image for nvidia-mig-manager
- Add MIG profiles for B300 and GB300 in the mig-parted config
- Bump NVIDIA Container Toolkit to v1.18.0
- Bump github.com/NVIDIA/go-nvlib from 0.7.4 to 0.8.1

## v0.12.3
- Use the correct MIG profiles for GB200
- Add MIG profiles for RTX PRO 6000 Blackwell
- Bump golang version to 1.24.6
- Bump go-nvlib to 0.7.4
- Update libxml2 and sqlite-libs packages in ubi9 image to resolve CVEs

## v0.12.2
- Add %posttrans to the rpm spec to ensure symlinks for config.yaml and hooks.yaml are maintained during an upgrade
- Bump golang version to v1.24.5
- Bump golang/x/net to v0.38.0
- Bump k8s.io/{api,api-machinery,client-go} dependencies to v0.33.3
- Bump CUDA base image version to 12.9.1
- Bump NVIDIA Container Toolkit to v1.17.8
- Bump go-nvlib to 0.7.3
- Bump go-nvml to 0.12.9-0

## v0.12.1
- Add the 4g.90gb MIG profile for the B200 GPU
- Bump golang/x/net to v0.36.0
- Bump k8s.io/{api,api-machinery,client-go} dependencies to v0.32.3
- Bump CUDA base image version to 12.8.1
- Bump NVIDIA Container Toolkit to v1.17.5

## v0.12.0
- Add support for the HGX GB200 GPU with PCI Device ID 2941
- Bump golang version to v1.24.1
- Bump go-nvlib to 0.7.1
- Bump k8s.io/{api,api-machinery,client-go} dependencies to v0.32.2
- Bump matryer/moq to v0.5.3
- Bump github.com/urfave/cli/v2 to 2.27.6

## v0.11.0
- Add support for the B200 GPU with PCI Device ID 2901
- Bump golang version to v1.23.5
- Bump golang/x/net to v0.33.0
- Bump nvidia-ctk version to v1.17.4
- Bump CUDA base image version to 12.8.0
- Bump go-nvml to 0.12.4-1
- Bump k8s.io/{api,api-machinery,client-go} dependencies to v0.32.1
- Bump matryer/moq to v0.5.1 
- Fix rpm spec to maintain the config.yaml and hooks.yaml symlinks during an update
- Point the QEMU artifacts image to the most recent tag - `tonistiigi/binfmt:master`

## v0.10.0
- Add GH200 144G HBM3e with PCI ID x234810DE
- Bump Golang version to v1.23.2
- Bump CUDA base image version to 12.6.2
- Switch to the UBI9 CUDA base image
- Bump nvidia-ctk version to v1.16.2
- Bump go-nvlib to 0.7.0
- Bump vendored go dependencies

## v0.9.1
- Add H200 with PCI ID 0x233510DE
- Fix bug with nvidia-gpu-reset.target setup
- Bump Golang version to v1.23.1
- Bump CUDA base image version to 12.6.1

## v0.9.0
- Bump go-nvlib to 0.6.1
- Fix the H100 NVL mig profile configuration in the example file
- Add arm64 packages
- Add a new nvidia-gpu-services.target to ensure proper startup order
- Add H800 with PCI ID 232410DE
- Bump Golang version to 1.23.0
- Bump nvidia-ctk version to v1.16.1
- Bump CUDA base image version to 12.6.0

## v0.8.0
- Add H200 141GB to the example config file
- Bump CUDA base image version to 12.5.1

## v0.8.0-rc.2
- Add --nvidia-cdi-hook flag to mig-manager
- Bump nvidia-ctk version to v1.16.0-rc.2
- Bump Golang version to 1.22.5

## v0.8.0-rc.1
- Update to latest CUDA base image 12.5.0
- Bump Golang version to 1.22.4
- Bump go-nvml to 0.12.0-5
- Bump go-nvlib to 0.3.0
- Update to incorporate go-nvml updates to expose interface types
- Align nvidia driver root envvar names with other components
- Add support for H100 NVL 94GB
- Add dev-root option to mig-manager container
- Bump nvidia-ctk version to v1.16.0-rc.1

## v0.7.0
- Update to latest CUDA base image 12.4.1
- Bump Golang version to 1.22.2
- Update vendored go dependencies

## v0.6.0
- Update to latest CUDA base image 12.3.2
- Migrate to using github.com/NVIDIA/go-nvlib
- Bump Golang version to 1.20.5
- Bump nvidia-ctk version used by k8s-mig-manager to 1.14.6
- Update vendored go dependencies
- Minor code improvements and refactoring

## v0.5.5
- Update to latest CUDA base image 12.2.2

## v0.5.4
- Update MIG config for Hopper with device ID of H100 80GB HBM3 SKU

## v0.5.3
- Update to latest CUDA image 12.2.0
- Update example config for Hopper with H100 NVL and H800 NVL

## v0.5.2
- Update to latest CUDA image 12.1.0
- Update k8s-mig-manager to support CDI
- Add two new example configs for the newly supported profiles on A100
- Update MIG profile code to rely on go-nvlib
- Update vendored go-nvlib to latest
- Update NVML wrapper to include MIG profiles from NVML v12.0

## v0.5.1
- Update to latest CUDA image 12.0.1
- Add newer MIG profiles supported with NVML 12.0 to default config.yaml files
- Add profiles with media extensions for A30-24GB to default config.yaml files
- Add H100 and H800 profiles to default config.yaml files
- Add A800 profiles to default config.yaml files
- Update all calls to enumerate GPUs to use NVML or PCI as appropriate
- Bump vendored go-nvml to v12.0
- Bump Golang version to 1.20.1

## v0.5.0
- Bump CUDA base image to 11.7.1
- Remove CUDA compat libs from mig-manager in favor of libs installed by the Driver
- Use symlink for config.yaml instead of static config file
- Add k8s-mig-manager-example for Hopper
- Update k8s-mig-manager-example with standalone RBAC objects
- Explicitly delete pods launched by operator validator before reconfig
- Allow missing GPUClients file in k8s-mig-manager
- Add hooks-minimal.yaml that gets linked if on Hopper or above
- Use symlink for hooks.yaml instead of static config file
- Update install script to use go 1.16.4
- Update hooks.sh to split out start/stop of k8s services from k8s pods
- Explicitly clear all MIG configurations before disabling MIG mode

## v0.4.3
- Update calculation for GB in MIG profile name
- Make the systemd-mig-manager a dependency of systemd-resolved.service

## v0.4.2
- Update CUDA image to 11.7.0
- Add extra assert in k8s-mig-manager to double check mig-mode change applied
- Update mig-manager image to use NGC DL license

## v0.4.1
- Keep NVML alive across all mig-parted commands (except GPU reset)
- Remove unnecessary services from hooks.sh

## v0.4.0
- Update nvidia-mig-parted.sh to include MIG_PARTED_CHECKPOINT_FILE
- Add checkpoint / restore commands to mig-parted CLI
- Update golang version to 1.16.4
- Support instantiation of *_PROFILE_6_SLICE GIs and CIs
- Update cyrus-sasl-lib to address CVE-2022-24407
- Add support for MIG profiles with +me as an attribute extension
- Support Compute Instances in mig-parted config such that CI != GI
- Update go-nvml to v0.11.6
- Change semantics of 'all' to mean 'all-mig-capable' in mig-parted config

## v0.3.0
- k8s-mig-manager: Add support for multi-arch images
- k8s-mig-manager: Handle eviction of NVSM pod when applying MIG changes

## v0.2.0
- nvidia-mig-parted:   Support passing newer GI and CI profile enums on older drivers
- k8s-mig-manager:     Rename nvcr.io/nvidia to nvcr.io/nvidia/cloud-native
- k8s-mig-manager:     Add support for pre-installed drivers
- systemd-mig-manager: Update logic to remove 'containerd' containers in utils.sh
- systemd-mig-manager: Update logic to shutdown only active systemd services in list
- ci-infrastructure:   Rework build and CI to align with other projects
- ci-infrastructure:   Use pulse instead of contamer for scans

## v0.1.3
- Add default configs for the PG506-96GB card
- Remove CombinedMigManager and add wrappers for Mode/Config Managers
- Add a function to check the minimum NVML version required
- Add SystemGetNVMLVersion() to the NVML interface
- Fix small bug in assert logic for non MIG-capable GPUs

## v0.1.2
- Do not start nvidia-mig-manager.service when installing the .deb
- Restore lost assert_gpu_reset_available() function
- Add nvidia-dcgm.service to driver_services array
- Split dcgm, and dcgm-exporter in k8s-mig-manager

## v0.1.1
- Update packaged config.yaml to include more supported devices

## v0.1.0
- Initial release of rpm package for v0.1.0
