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

source ${CURRDIR}/utils-custom.sh

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

function nvidia-mig-manager::service::remove_modules() {
	local -n __modules="${1}"
	for m in ${__modules[@]}; do
		nvidia-mig-manager::service::assert_module_loaded "${m}"
		if [ "${?}" != "0" ]; then
			continue
		fi
		rmmod "${m}"
		if [ "${?}" != "0" ]; then
			return 1
		fi
	done
	return 0
}

function nvidia-mig-manager::service::insert_modules() {
	local -n __modules="${1}"
	for m in ${__modules[@]}; do
		modprobe ${m}
		if [ "${?}" != "0" ]; then
			return 1
		fi
	done
	return 0
}

function nvidia-mig-manager::service::kill_k8s_containers_via_runtime_by_image() {
	nvidia-mig-manager::service::kill_k8s_containers_via_docker_by_image "${1}"
	if [ "${?}" != "0" ]; then
		return 1
	fi
	nvidia-mig-manager::service::kill_k8s_containers_via_containerd_by_image "${1}"
	if [ "${?}" != "0" ]; then
		return 1
	fi
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
		images+=("$(ctr -n k8s.io image ls | grep "${i}" | tr -s ' ' | cut -d' ' -f3 | tr '\n' ' ')")
	done

	for i in ${images[@]}; do
		local containers="$(ctr -n k8s.io container ls "image~=${i}" -q)"
		if [ "${containers}" != "" ]; then
			ctr -n k8s.io tasks kill -a -s SIGKILL ${containers}
			if [ "${?}" != "0" ]; then
				return 1
			fi
			sleep 10
			ctr -n k8s.io container rm ${containers}
			if [ "${?}" != "0" ]; then
				return 1
			fi
		fi
	done

	return 0
}

function nvidia-mig-manager::service::apply_mode() {
	local config_file="${1}"
	local selected_config="${2}"

	nvidia-mig-parted assert --mode-only -f "${config_file}" -c "${selected_config}"
	if [ "${?}" == "0" ]; then
		nvidia-mig-manager::service::post_apply_mode
		if [ "${?}" != "0" ]; then
			(set +x; echo "There was an error running post-apply-mode")
			return 1
		fi
		return 0
	fi

	local attempt=0
	local total=10
	(set +x; echo "Attempting to apply MIG mode setting with GPU reset (will try up to ${total} times)")
	until [ ${attempt} -ge ${total} ]; do
		attempt=$((attempt+1))
		(set +x; echo "Attempt ${attempt} of ${total}")

		if [ "${attempt}" != 1 ]; then
			sleep "$((attempt*30))"
		fi

		nvidia-mig-manager::service::assert_gpu_reset_available
		if [ "${?}" != "0" ]; then
			nvidia-mig-parted apply --mode-only --skip-reset -f "${config_file}" -c "${selected_config}"
			if [ "${?}" != "0" ]; then
				(set +x; echo "There was an error setting the desired MIG mode to pending")
				continue
			fi

			(set +x;
			echo "There is no GPU reset available to complete the MIG mode change"
			echo "A reboot is required to apply it")

			return 1
		fi

		nvidia-mig-manager::service::pre_apply_mode
		if [ "${?}" != "0" ]; then
			(set +x; echo "There was an error running pre-apply-mode")
			continue
		fi

		nvidia-mig-parted apply --mode-only -f "${config_file}" -c "${selected_config}"
		if [ "${?}" != "0" ]; then
			(set +x; echo "There was an error resetting the GPUs to activate the desired MIG mode")
			continue
		fi

		nvidia-mig-manager::service::post_apply_mode
		if [ "${?}" != "0" ]; then
			(set +x; echo "There was an error running post-apply-mode")
			continue
		fi

		return 0
	done

	return 1
}

function nvidia-mig-manager::service::apply_config() {
	local config_file="${1}"
	local selected_config="${2}"

	nvidia-mig-parted assert --mode-only -f "${config_file}" -c "${selected_config}"
	if [ "${?}" != "0" ]; then
		(set +x; echo "The MIG mode of the desired config must already be applied when running this script")
		return 1
	fi

	nvidia-mig-manager::service::assert_module_loaded "nvidia"
	if [ "${?}" != "0" ]; then
		(set +x: echo "The nvidia module must be loaded to apply a MIG config with this script")
		return 1
	fi

	nvidia-mig-parted assert -f "${config_file}" -c "${selected_config}"
	if [ "${?}" == "0" ]; then
		nvidia-mig-manager::service::post_apply_config
		if [ "${?}" != "0" ]; then
			(set +x; echo "There was an error running post-apply-config")
			return 1
		fi
		return 0
	fi

	local attempt=0
	local total=10
	(set +x; echo "Attempting to apply MIG config (will try up to ${total} times)")
	until [ ${attempt} -ge ${total} ]; do
		attempt=$((attempt+1))
		(set +x; echo "Attempt ${attempt} of ${total}")

		if [ "${attempt}" != 1 ]; then
			sleep "$((attempt*30))"
		fi

		nvidia-mig-manager::service::pre_apply_config
		if [ "${?}" != "0" ]; then
			(set +x; echo "There was an error running pre-apply-config")
			continue
		fi
		nvidia-mig-parted apply -f "${config_file}" -c "${selected_config}"
		if [ "${?}" != "0" ]; then
			(set +x; echo "There was an error applying the desired MIG config")
			continue
		fi
		nvidia-mig-manager::service::post_apply_config
		if [ "${?}" != "0" ]; then
			(set +x; echo "There was an error running post-apply-config")
			continue
		fi

		return 0
	done

	return 1
}
