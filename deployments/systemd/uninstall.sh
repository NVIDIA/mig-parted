#!/usr/bin/env bash

SERVICE_ROOT="nvidia-mig-manager"
SERVICE_NAME="${SERVICE_ROOT}.service"

MIG_PARTED_NAME="nvidia-mig-parted"
MIG_PARTED_GO_GET_PATH="github.com/NVIDIA/mig-parted/cmd/${MIG_PARTED_NAME}"

BINARY_DIR="/usr/bin/"
SYSTEMD_DIR="/usr/lib/systemd/system"
DATA_DIR="/var/lib/${SERVICE_ROOT}"
CONFIG_DIR="/etc/${SERVICE_ROOT}"
OVERRIDE_DIR="/etc/systemd/system/${SERVICE_NAME}.d"

systemctl disable ${SERVICE_NAME}
systemctl daemon-reload

rm -rf ${DATA_DIR}
rm -rf ${CONFIG_DIR}
rm -rf ${OVERRIDE_DIR}

rm ${BINARY_DIR}/${MIG_PARTED_NAME}
rm ${SYSTEMD_DIR}/${SERVICE_NAME}
