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

mkdir -p ${BINARY_DIR}
mkdir -p ${SYSTEMD_DIR}
mkdir -p ${DATA_DIR}
mkdir -p ${CONFIG_DIR}
mkdir -p ${OVERRIDE_DIR}

GO111MODULE=off     go get -u ${MIG_PARTED_GO_GET_PATH}
GOBIN=${BINARY_DIR} go install ${MIG_PARTED_GO_GET_PATH}

cp ${SERVICE_NAME} ${SYSTEMD_DIR}
cp override.conf   ${OVERRIDE_DIR}
cp service.sh      ${CONFIG_DIR}
cp utils.sh        ${CONFIG_DIR}
cp utils-custom.sh ${CONFIG_DIR}
cp apply-config.sh ${CONFIG_DIR}
cp config.yaml     ${CONFIG_DIR}

systemctl daemon-reload
systemctl enable ${SERVICE_NAME}
