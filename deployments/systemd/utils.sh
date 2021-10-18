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

function nvidia-mig-manager::service::reverse_array() {
	# first argument is the array to reverse
	# second is the reversed array
	local -n arr="${1}"
	local -n rev="${2}"
	for i in "${arr[@]}"; do
		rev=("${i}" "${rev[@]}")
	done
}

function nvidia-mig-manager::service::assert_module_loaded() {
   local module="${1}"
   cat /proc/modules | grep -e "^${module} "
   if [ "${?}" == "0" ]; then
       return 0
   fi
   return 1
}

function nvidia-mig-manager::service::assert_gpu_reset_available() {
	local devices_path="/sys/bus/pci/devices"
	for d in $(ls "${devices_path}"); do
		local vendor="$(cat "${devices_path}/${d}/vendor")"
		if [ "${vendor}" != "0x10de" ]; then
			continue
		fi
		local class="$(cat "${devices_path}/${d}/class")"
		if [ "${class}" != "0x030200" ]; then
			continue
		fi
		if [ ! -f "${devices_path}/${d}/reset" ]; then
			return 1
		fi
	done
	return 0
}

function nvidia-mig-manager::service::reboot() {
	local statedir="/var/lib/nvidia-mig-manager"
	mkdir -p "${statedir}"

	if [ ! -f "${statedir}/reboot_attempted" ]; then
		touch "${statedir}/reboot_attempted"
		reboot
		return ${?}
	fi

	(set +x;
	echo "Machine already rebooted once -- not attempting again"
	echo "You must manually remove the following file to enable automatic reboots again:"
	echo "    ${statedir}/reboot_attempted")
	return 1
}

function nvidia-mig-manager::service::clear_reboot_state() {
	local statedir="/var/lib/nvidia-mig-manager"
	rm -rf "${statedir}/reboot_attempted"
}

function nvidia-mig-manager::service::persist_config_across_reboot() {
	local selected_config="${1}"
cat << EOF > /etc/systemd/system/nvidia-mig-manager.service.d/override.conf
[Service]
Environment="MIG_PARTED_SELECTED_CONFIG=${selected_config}"
EOF
	systemctl daemon-reload
}

function nvidia-mig-manager::service::start_systemd_services() {
	local -n __services="${1}"
	for s in ${__services[@]}; do
		systemctl list-unit-files --state=enabled,generated | grep -F "${s}"
		if [ "${?}" != "0" ]; then
			continue
		fi
		systemctl start "${s}"
		if [ "${?}" != "0" ]; then
			return 1
		fi
	done
	return 0
}

function nvidia-mig-manager::service::stop_systemd_services() {
	local -n __services="${1}"
	for s in ${__services[@]}; do
		systemctl list-unit-files --state=enabled,generated | grep -F "${s}"
		if [ "${?}" != "0" ]; then
			continue
		fi
		systemctl stop "${s}"
		if [ "${?}" != "0" ]; then
			return 1
		fi
	done
	return 0
}

function nvidia-mig-manager::service::kill_k8s_containers_via_docker_by_image() {
	local images=()
	local -n __image_names="${1}"

	for i in ${__image_names[@]}; do
		images+=("${i}")
		images+=("$(docker images --format "{{.ID}} {{.Repository}}" | grep "${i}" | cut -d' ' -f1 | tr '\n' ' ')")
	done

	for i in ${images[@]}; do
		local containers="$(docker ps --format "{{.ID}} {{.Image}}" | grep "${i}" | cut -d' ' -f1 | tr '\n' ' ')"
		if [ "${containers}" != "" ]; then
			docker kill ${containers}
			if [ "${?}" != "0" ]; then
				return 1
			fi
			sleep 10
			docker rm ${containers}
			if [ "${?}" != "0" ]; then
				return 1
			fi
		fi
	done

	return 0
}

function nvidia-mig-manager::service::kill_k8s_containers_via_containerd_by_image() {
	local images=()
	local -n __image_names="${1}"

	for i in ${__image_names[@]}; do
		images+=("${i}")
		images+=("$(ctr -n k8s.io image ls | grep "${i}" | tr -s ' ' | cut -d' ' -f1 | tr '\n' ' ')")
	done

	for i in ${images[@]}; do
		local containers="$(ctr -n k8s.io container ls "image~=${i}" -q)"
		if [ "${containers}" != "" ]; then
			ctr -n k8s.io task kill -a -s SIGKILL ${containers} || true
			if [ "${?}" != "0" ]; then
				return 1
			fi
			sleep 10
			ctr -n k8s.io task rm -f ${containers} || true
			if [ "${?}" != "0" ]; then
				return 1
			fi
			ctr -n k8s.io container rm ${containers}
			if [ "${?}" != "0" ]; then
				return 1
			fi
		fi
	done

	return 0
}
