#!/usr/bin/env bash

# Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

CURRDIR="$(cd "$( dirname $(readlink -f "${BASH_SOURCE[0]}"))" >/dev/null 2>&1 && pwd)"

source ${CURRDIR}/utils.sh

driver_services=(
	nvsm.service
	nvidia-dcgm.service
)

k8s_services=(
	dcgm-exporter.service
	kubelet.service
)

k8s_pod_images=(
	k8s-device-plugin
	gpu-feature-discovery
	dcgm-exporter
)

function apply-start() {
	local selected_config="${1}"
	nvidia-mig-manager::service::persist_config_across_reboot ${selected_config}
	if [ "${?}" != "0" ]; then
		return 1
	fi
	return 0
}

function pre-apply-mode() {
	stop_k8s_services
	if [ "${?}" != "0" ]; then
		return 1
	fi
	stop_k8s_pods
	if [ "${?}" != "0" ]; then
		return 1
	fi
	stop_driver_services
	if [ "${?}" != "0" ]; then
		return 1
	fi
	return 0
}

function pre-apply-config() {
	stop_k8s_services
	if [ "${?}" != "0" ]; then
		return 1
	fi
	stop_k8s_pods
	if [ "${?}" != "0" ]; then
		return 1
	fi
	return 0
}

function apply-exit() {
	start_driver_services
	if [ "${?}" != "0" ]; then
		return 1
	fi
	start_k8s_services
	if [ "${?}" != "0" ]; then
		return 1
	fi
}

function stop_driver_services() {
	local services=()
	nvidia-mig-manager::service::reverse_array \
		driver_services \
		services
	nvidia-mig-manager::service::stop_systemd_services services
	return ${?}
}

function start_driver_services() {
	# Driver services are Wants=/After=nvidia-gpu-reset.target and this
	# service runs Before= that target, so a synchronous start from this hook
	# can wait on a job that is itself waiting for this service to finish.
	# This happens whenever the target is not yet active: during boot, during
	# package installation, or on the first start after install. Never wait:
	# enqueue the start and return; systemd runs the queued starts once this
	# service completes and the target activates. Startup failures are
	# reported by the started units themselves, not by this hook; a non-zero
	# return here means systemd refused to enqueue the request.
	nvidia-mig-manager::service::start_systemd_services driver_services "--no-block"
	return ${?}
}

function stop_k8s_services() {
	local services=()
	nvidia-mig-manager::service::reverse_array \
		k8s_services \
		services
	nvidia-mig-manager::service::stop_systemd_services services
	return ${?}
}

function start_k8s_services() {
	# Same policy as start_driver_services: never wait on a systemd job from
	# inside this hook, in case any of these units is ordered (directly or
	# transitively) after nvidia-gpu-reset.target.
	nvidia-mig-manager::service::start_systemd_services k8s_services "--no-block"
	return ${?}
}

function stop_k8s_pods() {
	nvidia-mig-manager::service::kill_k8s_containers_via_docker_by_image k8s_pod_images
	if [ "${?}" != "0" ]; then
		return 1
	fi
	nvidia-mig-manager::service::kill_k8s_containers_via_containerd_by_image k8s_pod_images
	if [ "${?}" != "0" ]; then
		return 1
	fi
	return 0
}

# refresh-cdi triggers the nvidia-cdi-refresh service to regenerate CDI
# specifications, making updated GPU devices available to container runtimes.
function refresh-cdi() {
    # Check if nvidia-cdi-refresh.service exists
    if systemctl list-unit-files nvidia-cdi-refresh.service --quiet; then
        echo "Found nvidia-cdi-refresh.service, calling systemctl..." >&2
        if ! systemctl restart nvidia-cdi-refresh.service; then
            echo "Error: Failed to start nvidia-cdi-refresh.service" >&2
        fi
    fi
}
