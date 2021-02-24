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
