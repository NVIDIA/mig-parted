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

SERVICE_ROOT="nvidia-mig-manager"
SERVICE_NAME="${SERVICE_ROOT}.service"

MIG_PARTED_NAME="nvidia-mig-parted"
MIG_PARTED_GO_GET_PATH="github.com/NVIDIA/mig-parted/cmd/${MIG_PARTED_NAME}"

BINARY_DIR="/usr/bin"
SYSTEMD_DIR="/usr/lib/systemd/system"
DATA_DIR="/var/lib/${SERVICE_ROOT}"
CONFIG_DIR="/etc/${SERVICE_ROOT}"
OVERRIDE_DIR="/etc/systemd/system/${SERVICE_NAME}.d"
PROFILED_DIR="/etc/profile.d"

systemctl disable ${SERVICE_NAME}
systemctl daemon-reload

rm -rf ${DATA_DIR}
rm -rf ${CONFIG_DIR}
rm -rf ${OVERRIDE_DIR}

rm ${BINARY_DIR}/${MIG_PARTED_NAME}
rm ${SYSTEMD_DIR}/${SERVICE_NAME}
rm ${PROFILED_DIR}/mig-parted.sh
