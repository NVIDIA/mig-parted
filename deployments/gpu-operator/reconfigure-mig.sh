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

WITH_REBOOT="false"
HOST_ROOT_MOUNT="/host"
NODE_NAME=""
MIG_CONFIG_FILE=""
SELECTED_MIG_CONFIG=""

function usage() {
  echo "USAGE:"
  echo "    ${0} -h "
  echo "    ${0} -n <node> -f <config-file> -c <selected-config> [ -r ]"
  echo ""
  echo "OPTIONS:"
  echo "    -h                   Display this help message"
  echo "    -r                   Automatically reboot the node if changing the MIG mode fails for any reason"
  echo "    -n <node>            The kubernetes node to change the MIG configuration on"
  echo "    -f <config-file>     The mig-parted configuration file"
  echo "    -c <selected-config> The selected mig-parted configuration to apply to the node"
  echo "    -m <host-root-mount> Target path where host root directory is mounted"
}

while getopts "hrn:f:c:m:" opt; do
  case ${opt} in
    h ) # process option h
      usage; exit 0
      ;;
    r ) # process option r
      WITH_REBOOT="true"
      ;;
    n ) # process option n
      NODE_NAME=${OPTARG}
      ;;
    f ) # process option f
      MIG_CONFIG_FILE=${OPTARG}
      ;;
    c ) # process option c
      SELECTED_MIG_CONFIG=${OPTARG}
      ;;
    m ) # process option m
      HOST_ROOT_MOUNT=${OPTARG}
      ;;
    \? ) echo "Usage: ${0} -n <node> -f <config-file> -c <selected-config> [ -m <host-root-mount> -r ]"
      ;;
  esac
done

if [ "${NODE_NAME}" = "" ]; then
  echo "ERROR: missing -n <node> flag"
  usage; exit 1
fi
if [ "${MIG_CONFIG_FILE}" = "" ]; then
  echo "Error: missing -f <config-file> flag"
  usage; exit 1
fi
if [ "${SELECTED_MIG_CONFIG}" = "" ]; then
  echo "Error: missing -c <selected-config> flag"
  usage; exit 1
fi

function __set_state_and_exit() {
	local state="${1}"
	local exit_code="${2}"

	echo "Changing the 'nvidia.com/mig.config.state' node label to '${state}'"
	kubectl label --overwrite  \
		node ${NODE_NAME} \
		nvidia.com/mig.config.state="${state}"
	if [ "${?}" != "0" ]; then
		echo "Unable to set 'nvidia.com/mig.config.state' to \'${state}\'"
		echo "Exiting with incorrect value in 'nvidia.com/mig.config.state'"
		exit 1
	fi

	exit ${exit_code}
}	

function exit_success() {
	__set_state_and_exit "success" 0
}	

function exit_failed_no_restart_gpu_clients() {
	__set_state_and_exit "failed" 1
}

function exit_failed() {
	echo "Restarting all GPU clients previouly shutdown by reenabling their component-specific nodeSelector labels"
	kubectl label --overwrite \
		node ${NODE_NAME} \
		nvidia.com/gpu.deploy.device-plugin=$(maybe_set_true ${PLUGIN_DEPLOYED}) \
		nvidia.com/gpu.deploy.gpu-feature-discovery=$(maybe_set_true ${GFD_DEPLOYED}) \
		nvidia.com/gpu.deploy.dcgm-exporter=$(maybe_set_true ${DCGM_EXPORTER_DEPLOYED}) \
		nvidia.com/gpu.deploy.dcgm=$(maybe_set_true ${DCGM_DEPLOYED})
		if [ "${?}" != "0" ]; then
			echo "Unable to bring up GPU operator components by setting their daemonset labels"
		fi
	__set_state_and_exit "failed" 1
}

# Only return 'paused-*' if the value passed in is != 'false'. It should only
# be 'false' if some external entity has forced it to this value, at which point
# we want to honor it's existing value and not change it.
function maybe_set_paused() {
	local current_value="${1}"
	if [  "${current_value}" = "false" ]; then
		echo "false"
	else
		echo "paused-for-mig-change"
	fi
}

# Only return 'true' if the value passed in is != 'false'. It should only
# be 'false' if some external entity has forced it to this value, at which point
# we want to honor it's existing value and not change it.
function maybe_set_true() {
	local current_value="${1}"
	if [  "${current_value}" = "false" ]; then
		echo "false"
	else
		echo "true"
	fi
}

echo "Getting current value of the 'nvidia.com/gpu.deploy.device-plugin' node label"
PLUGIN_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.device-plugin}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.device-plugin' label"
	exit_failed_no_restart_gpu_clients
fi
echo "Current value of 'nvidia.com/gpu.deploy.device-plugin=${PLUGIN_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.gpu-feature-discovery' node label"
GFD_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.gpu-feature-discovery}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.gpu-feature-discovery' label"
	exit_failed_no_restart_gpu_clients
fi
echo "Current value of 'nvidia.com/gpu.deploy.gpu-feature-discovery=${GFD_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.dcgm-exporter' node label"
DCGM_EXPORTER_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.dcgm-exporter}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.dcgm-exporter' label"
	exit_failed_no_restart_gpu_clients
fi
echo "Current value of 'nvidia.com/gpu.deploy.dcgm-exporter=${DCGM_EXPORTER_DEPLOYED}'"

echo "Getting current value of the 'nvidia.com/gpu.deploy.dcgm' node label"
DCGM_DEPLOYED=$(kubectl get nodes ${NODE_NAME} -o=jsonpath='{$.metadata.labels.nvidia\.com/gpu\.deploy\.dcgm}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/gpu.deploy.dcgm' label"
	exit_failed_no_restart_gpu_clients
fi
echo "Current value of 'nvidia.com/gpu.deploy.dcgm=${DCGM_DEPLOYED}'"

echo "Asserting that the requested configuration is present in the configuration file"
nvidia-mig-parted assert --valid-config -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
if [ "${?}" != "0" ]; then
	echo "Unable to validate the selected MIG configuration"
	exit_failed
fi

echo "Getting current value of the 'nvidia.com/mig.config.state' node label"
STATE=$(kubectl get node "${NODE_NAME}" -o=jsonpath='{.metadata.labels.nvidia\.com/mig\.config\.state}')
if [ "${?}" != "0" ]; then
	echo "Unable to get the value of the 'nvidia.com/mig.config.state' label"
	exit_failed
fi
echo "Current value of 'nvidia.com/mig.config.state=${STATE}'"

echo "Checking if the MIG mode setting in the selected config is currently applied or not"
echo "If the state is 'rebooting', we expect this to always return true"
nvidia-mig-parted assert --mode-only -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
if [ "${?}" != "0" ] && [ "${STATE}" == "rebooting" ]; then
	echo "MIG mode change did not take effect after rebooting"
	exit_failed
fi

echo "Checking if the selected MIG config is currently applied or not"
nvidia-mig-parted assert -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
if [ "${?}" = "0" ]; then
	exit_success
fi

echo "Changing the 'nvidia.com/mig.config.state' node label to 'pending'"
kubectl label --overwrite  \
	node ${NODE_NAME} \
	nvidia.com/mig.config.state="pending"
if [ "${?}" != "0" ]; then
	echo "Unable to set the value of 'nvidia.com/mig.config.state' to 'pending'"
	exit_failed
fi

echo "Shutting down all GPU clients on the current node by disabling their component-specific nodeSelector labels"
kubectl label --overwrite \
	node ${NODE_NAME} \
	nvidia.com/gpu.deploy.device-plugin=$(maybe_set_paused ${PLUGIN_DEPLOYED}) \
	nvidia.com/gpu.deploy.gpu-feature-discovery=$(maybe_set_paused ${GFD_DEPLOYED}) \
	nvidia.com/gpu.deploy.dcgm-exporter=$(maybe_set_paused ${DCGM_EXPORTER_DEPLOYED}) \
	nvidia.com/gpu.deploy.dcgm=$(maybe_set_paused ${DCGM_DEPLOYED})
if [ "${?}" != "0" ]; then
	echo "Unable to tear down GPU operator components by setting their daemonset labels"
	exit_failed
fi

echo "Waiting for the device-plugin to shutdown"
kubectl wait --for=delete pod \
	--timeout=5m \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n gpu-operator-resources \
	-l app=nvidia-device-plugin-daemonset

echo "Waiting for gpu-feature-discovery to shutdown"
kubectl wait --for=delete pod \
	--timeout=5m \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n gpu-operator-resources \
	-l app=gpu-feature-discovery

echo "Waiting for dcgm-exporter to shutdown"
kubectl wait --for=delete pod \
	--timeout=5m \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n gpu-operator-resources \
	-l app=nvidia-dcgm-exporter

echo "Waiting for dcgm to shutdown"
kubectl wait --for=delete pod \
	--timeout=5m \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n gpu-operator-resources \
	-l app=nvidia-dcgm

echo "Applying the MIG mode change from the selected config to the node"
echo "If the -r option was passed, the node will be automatically rebooted if this is not successful"
nvidia-mig-parted -d apply --mode-only -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
if [ "${?}" != "0" ] && [ "${WITH_REBOOT}" = "true" ]; then
	echo "Changing the 'nvidia.com/mig.config.state' node label to 'rebooting'"
	kubectl label --overwrite  \
		node ${NODE_NAME} \
		nvidia.com/mig.config.state="rebooting"
	if [ "${?}" != "0" ]; then
		echo "Unable to set the value of 'nvidia.com/mig.config.state' to 'rebooting'"
		echo "Exiting so as not to reboot multiple times unexpectedly"
		exit_failed
	fi
	chroot ${HOST_ROOT_MOUNT} reboot
	exit 0
fi

echo "Applying the selected MIG config to the node"
nvidia-mig-parted -d apply -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
if [ "${?}" != "0" ]; then
	exit_failed
fi

echo "Restarting all GPU clients previouly shutdown by reenabling their component-specific nodeSelector labels"
kubectl label --overwrite \
	node ${NODE_NAME} \
	nvidia.com/gpu.deploy.device-plugin=$(maybe_set_true ${PLUGIN_DEPLOYED}) \
	nvidia.com/gpu.deploy.gpu-feature-discovery=$(maybe_set_true ${GFD_DEPLOYED}) \
	nvidia.com/gpu.deploy.dcgm-exporter=$(maybe_set_true ${DCGM_EXPORTER_DEPLOYED}) \
	nvidia.com/gpu.deploy.dcgm=$(maybe_set_true ${DCGM_DEPLOYED})
if [ "${?}" != "0" ]; then
	echo "Unable to bring up GPU operator components by setting their daemonset labels"
	exit_failed_no_restart_gpu_clients
fi

echo "Restarting validator pod to re-run all validations"
kubectl delete pod \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n gpu-operator-resources \
	-l app=nvidia-operator-validator

exit_success
