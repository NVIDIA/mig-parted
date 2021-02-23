#!/usr/bin/env bash

CURRDIR="$(cd "$( dirname $(readlink -f "${BASH_SOURCE[0]}"))" >/dev/null 2>&1 && pwd)"

source ${CURRDIR}/utils.sh

: "${MIG_PARTED_CONFIG_FILE:=${CURRDIR}/config.yaml}"
: "${MIG_PARTED_SELECTED_CONFIG:?Environment variable must be set before calling this script}"

export MIG_PARTED_CONFIG_FILE
export MIG_PARTED_SELECTED_CONFIG

set -x

# Check if the desired MIG mode is already applied
nvidia-mig-parted assert --mode-only

# If it is not, then go through the process of applying it
if [ "${?}" != 0 ]; then
	# Apply MIG mode, without issuing a GPU reset
    nvidia-mig-parted apply --mode-only --skip-reset
	if [ "${?}" != 0 ]; then
    	(set +x; echo "Error applying MIG mode")
		exit 1
	fi

	# If GPU reset is not available (e.g. GPU passthrough virtualization),
	# then issue a reboot. The reboot will only occur once. If the MIG mode is
	# still not applied after reboot, this script will error out.
	nvidia-mig-manager::service::assert_gpu_reset_available
	if [ "${?}" != 0 ]; then
    	(set +x;
		echo "GPU reset capabilities are not available"
    	echo "Attempting reboot")
		nvidia-mig-manager::service::reboot
		exit "${?}"
	fi

 	# Since the desired MIG mode is already applied, the
	# following will just do a GPU reset under the hood
    nvidia-mig-parted apply --mode-only
	if [ "${?}" != 0 ]; then
    	(set +x; echo "Error issuing GPU reset")
		exit 1
	fi
fi

# In case a reboot was issued by a previous iteration of this script, we clear
# the reboot state so that the next next MIG mode change + reboot will succeed.
nvidia-mig-manager::service::clear_reboot_state

nvidia-mig-manager::service::assert_module_loaded "nvidia"
if [ "${?}" != 0 ]; then
    (set +x; echo "No nvidia module loaded, skipping MIG device config")
	exit 0
fi

nvidia-mig-parted apply
if [ "${?}" != 0 ]; then
	(set +x; echo "Error applying MIG config")
	exit 1
fi

nvidia-mig-parted export
