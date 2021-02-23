#!/usr/bin/env bash

function nvidia-mig-manager::service::pre_apply_mode() {
	nvidia-mig-manager::service::stop_k8s_components
	if [ "${?}" != "0" ]; then
		return 1
	fi
	nvidia-mig-manager::service::stop_driver_services
	if [ "${?}" != "0" ]; then
		return 1
	fi
	nvidia-mig-manager::service::remove_driver_modules
	if [ "${?}" != "0" ]; then
		return 1
	fi
	return 0
}

function nvidia-mig-manager::service::post_apply_mode() {
	nvidia-mig-manager::service::insert_driver_modules
	if [ "${?}" != "0" ]; then
		return 1
	fi
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

function nvidia-mig-manager::service::remove_driver_modules() {
	local modules=(
		nvidia_uvm
		nvidia_drm
		nvidia_modeset
		nvidia
	)
	nvidia-mig-manager::service::remove_modules modules
	return ${?}
}

function nvidia-mig-manager::service::insert_driver_modules() {
	local modules=(
		nvidia
		nvidia_modeset
		nvidia_drm
		nvidia_uvm
	)
	nvidia-mig-manager::service::insert_modules modules
	return ${?}
}

function nvidia-mig-manager::service::stop_driver_services() {
	local services=(
		dcgm.service
		nv_peer_mem.service
		nvsm-notifier.service
		nvsm-api-gateway.service
		nvsm-core.service
		nvsm-mqtt.service
		nvsm.service
		nvidia-fabricmanager.service
		nvidia-persistenced.service
	)
	nvidia-mig-manager::service::stop_systemd_services services
	return ${?}
}

function nvidia-mig-manager::service::start_driver_services() {
	local services=(
		nvidia-persistenced.service
		nvidia-fabricmanager.service
		nvsm.service
		nvsm-mqtt.service
		nvsm-core.service
		nvsm-api-gateway.service
		nvsm-notifier.service
		nv_peer_mem.service
		dcgm.service
	)
	nvidia-mig-manager::service::start_systemd_services services
	return ${?}
}

function nvidia-mig-manager::service::stop_k8s_components() {
	local services=(
		kubelet.service
		dcgm-exporter.service
	)
	nvidia-mig-manager::service::stop_systemd_services services
	if [ "${?}" != "0" ]; then
		return 1
	fi
	local container_images=(
		k8s-device-plugin
		gpu-feature-discovery
	)
	nvidia-mig-manager::service::kill_k8s_containers_via_runtime_by_image container_images
	if [ "${?}" != "0" ]; then
		return 1
	fi
	return 0
}

function nvidia-mig-manager::service::start_k8s_components() {
	local services=(
		dcgm-exporter.service
		kubelet.service
	)
	nvidia-mig-manager::service::start_systemd_services services
	return ${?}
}
