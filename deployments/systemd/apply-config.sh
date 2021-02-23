#!/usr/bin/env bash

CURRDIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

source ${CURRDIR}/utils.sh

if [ "$#" != "1" ]; then
	(set +x; echo "Requires exactly one argument with the name of the desired MIG config")
	exit 1
fi

: "${config_file:=${CURRDIR}/config.yaml}"
: "${selected_config:=${1}}"

set -x

nvidia-mig-manager::service::persist_config_across_reboot "${selected_config}"
if [ "${?}" != "0" ]; then
    (set +x; echo "Error persisting config across reboots")
    exit 1
fi
nvidia-mig-manager::service::apply_mode "${config_file}" "${selected_config}"
if [ "$?" != 0 ]; then
    (set +x; echo "Error applying MIG mode")
	exit 1
fi
nvidia-mig-manager::service::apply_config "${config_file}" "${selected_config}"
if [ "$?" != 0 ]; then
    (set +x; echo "Error applying MIG config")
	exit 1
fi
nvidia-mig-parted export
