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

: ${DOCKER:=docker}

SERVICE_ROOT="nvidia-mig-manager"
SERVICE_NAME="${SERVICE_ROOT}.service"
MIG_PARTED_NAME="nvidia-mig-parted"

BINARY_DIR="/usr/bin"
SYSTEMD_DIR="/usr/lib/systemd/system"
DATA_DIR="/var/lib/${SERVICE_ROOT}"
CONFIG_DIR="/etc/${SERVICE_ROOT}"
OVERRIDE_DIR="/etc/systemd/system/${SERVICE_NAME}.d"
PROFILED_DIR="/etc/profile.d"

mkdir -p ${BINARY_DIR}
mkdir -p ${SYSTEMD_DIR}
mkdir -p ${DATA_DIR}
mkdir -p ${CONFIG_DIR}
mkdir -p ${OVERRIDE_DIR}
mkdir -p ${PROFILED_DIR}

chmod a+rx ${BINARY_DIR}
chmod a+rx ${SYSTEMD_DIR}
chmod a+rx ${DATA_DIR}
chmod a+rx ${CONFIG_DIR}
chmod a+rx ${OVERRIDE_DIR}
chmod a+rx ${PROFILED_DIR}

cp ${SERVICE_NAME}       ${SYSTEMD_DIR}
cp ${MIG_PARTED_NAME}    ${BINARY_DIR}
cp ${MIG_PARTED_NAME}.sh ${PROFILED_DIR}
cp override.conf         ${OVERRIDE_DIR}
cp service.sh            ${CONFIG_DIR}
cp utils.sh              ${CONFIG_DIR}
cp hooks.sh              ${CONFIG_DIR}
cp hooks-default.yaml    ${CONFIG_DIR}
cp hooks-minimal.yaml    ${CONFIG_DIR}
cp config-ampere.yaml    ${CONFIG_DIR}

chmod a+r ${SYSTEMD_DIR}/${SERVICE_NAME}
chmod a+r ${PROFILED_DIR}/${MIG_PARTED_NAME}.sh
chmod a+r ${OVERRIDE_DIR}/override.conf
chmod a+r ${CONFIG_DIR}/service.sh
chmod a+r ${CONFIG_DIR}/utils.sh
chmod a+r ${CONFIG_DIR}/hooks.sh
chmod a+r ${CONFIG_DIR}/hooks-default.yaml
chmod a+r ${CONFIG_DIR}/hooks-minimal.yaml
chmod a+r ${CONFIG_DIR}/config-ampere.yaml

chmod a+x ${BINARY_DIR}/${MIG_PARTED_NAME}
chmod ug+x ${CONFIG_DIR}/service.sh

systemctl daemon-reload
systemctl enable ${SERVICE_NAME}

function maybe_add_hooks_symlink() {
  if [ -e ${CONFIG_DIR}/hooks.yaml ]; then
    return
  fi

  which nvidia-smi > /dev/null 2>&1
  if [ "${?}" != 0 ]; then
    return
  fi

  local compute_cap=$(nvidia-smi -i 0 --query-gpu=compute_cap --format=csv,noheader)
  if [ "${compute_cap/./}" -ge "90" ]; then
    ln -s hooks-minimal.yaml ${CONFIG_DIR}/hooks.yaml
  else
    ln -s hooks-default.yaml ${CONFIG_DIR}/hooks.yaml
  fi
}

function maybe_add_config_symlink() {
  if [ -e ${CONFIG_DIR}/config.yaml ]; then
    return
  fi
  ln -s config-ampere.yaml ${CONFIG_DIR}/config.yaml
}

maybe_add_hooks_symlink
maybe_add_config_symlink
