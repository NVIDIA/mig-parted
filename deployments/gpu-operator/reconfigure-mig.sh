#!/usr/bin/env bash

WITH_REBOOT="false"
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
}

while getopts "hrn:f:c:d:" opt; do
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
    \? ) echo "Usage: ${0} -n <node> -f <config-file> -c <selected-config> [ -r ]"
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

function exit_failed() {
	__set_state_and_exit "failed" 1
}	

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
	nvidia.com/gpu.deploy.device-plugin=false \
	nvidia.com/gpu.deploy.gpu-feature-discovery=false \
	nvidia.com/gpu.deploy.dcgm-exporter=false \
	nvidia.com/gpu.deploy.operator-validator=false
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

echo "Waiting for operator-validator to shutdown"
kubectl wait --for=delete pod \
	--timeout=5m \
	--field-selector "spec.nodeName=${NODE_NAME}" \
	-n gpu-operator-resources \
	-l app=nvidia-operator-validator

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
	chroot /host reboot
	exit 0
fi

echo "Applying the selected MIG config to the node"
nvidia-mig-parted -d apply -f ${MIG_CONFIG_FILE} -c ${SELECTED_MIG_CONFIG}
apply_exit_code="${?}"

echo "Restarting all GPU clients previouly shutdown by reenabling their component-specific nodeSelector labels"
kubectl label --overwrite \
	node ${NODE_NAME} \
	nvidia.com/gpu.deploy.device-plugin=true \
	nvidia.com/gpu.deploy.gpu-feature-discovery=true \
	nvidia.com/gpu.deploy.dcgm-exporter=true \
	nvidia.com/gpu.deploy.operator-validator=true
if [ "${?}" != "0" ]; then
	echo "Unable to bring up GPU operator components by setting their daemonset labels"
	exit_failed
fi

if [ "${apply_exit_code}" != "0" ]; then
	exit_failed
fi

exit_success
