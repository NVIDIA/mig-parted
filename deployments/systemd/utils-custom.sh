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

custom_driver_services=(
	nvsm.service
	nvsm-mqtt.service
	nvsm-core.service
	nvsm-api-gateway.service
	nvsm-notifier.service
	nv_peer_mem.service
	dcgm.service
)

custom_k8s_services=(
	dcgm-exporter.service
	kubelet.service
)

custom_k8s_pods=(
	k8s-device-plugin
	gpu-feature-discovery
	dcgm-exporter
)

function nvidia-mig-manager::service::pre_apply_mode() {
	nvidia-mig-manager::service::stop_k8s_components
	if [ "${?}" != "0" ]; then
		return 1
	fi
	nvidia-mig-manager::service::stop_driver_services
	if [ "${?}" != "0" ]; then
		return 1
	fi
	return 0
}

function nvidia-mig-manager::service::post_apply_mode() {
	nvidia-mig-manager::service::start_driver_services
	if [ "${?}" != "0" ]; then
		return 1
	fi
	nvidia-mig-manager::service::start_k8s_components
	if [ "${?}" != "0" ]; then
		return 1
	fi
	return 0
}

function nvidia-mig-manager::service::pre_apply_config() {
	nvidia-mig-manager::service::stop_k8s_components
	return ${?}
}

function nvidia-mig-manager::service::post_apply_config() {
	nvidia-mig-manager::service::start_k8s_components
	return ${?}
}

function nvidia-mig-manager::service::stop_driver_services() {
	local services=()
	nvidia-mig-manager::service::reverse_array \
		custom_driver_services \
		services
	nvidia-mig-manager::service::stop_systemd_services services
	return ${?}
}

function nvidia-mig-manager::service::start_driver_services() {
	nvidia-mig-manager::service::start_systemd_services custom_driver_services
	return ${?}
}

function nvidia-mig-manager::service::stop_k8s_components() {
	local services=()
	nvidia-mig-manager::service::reverse_array \
		custom_k8s_services \
		services
	nvidia-mig-manager::service::stop_systemd_services services
	if [ "${?}" != "0" ]; then
		return 1
	fi

	nvidia-mig-manager::service::kill_k8s_containers_via_runtime_by_image custom_k8s_pods
	if [ "${?}" != "0" ]; then
		return 1
	fi

	return 0
}

function nvidia-mig-manager::service::start_k8s_components() {
	nvidia-mig-manager::service::start_systemd_services custom_k8s_services
	return ${?}
}
