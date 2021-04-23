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

ARG BASE_IMAGE=undefined
ARG GOLANG_VERSION=undefined
FROM golang:${GOLANG_VERSION} AS build

WORKDIR /build
COPY . .
RUN go build -o /artifacts/nvidia-mig-parted ./cmd
RUN go build -o /artifacts/nvidia-mig-manager ./deployments/gpu-operator/
RUN cp ./deployments/gpu-operator/reconfigure-mig.sh /artifacts/reconfigure-mig.sh

RUN curl -o /artifacts/kubectl -L "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl";
RUN chmod a+x /artifacts/kubectl

FROM ${BASE_IMAGE}

COPY ./LICENSE ./licenses/LICENSE
COPY --from=build /artifacts/nvidia-mig-parted  /usr/bin/nvidia-mig-parted
COPY --from=build /artifacts/nvidia-mig-manager /usr/bin/nvidia-mig-manager
COPY --from=build /artifacts/reconfigure-mig.sh /usr/bin/reconfigure-mig.sh
COPY --from=build /artifacts/kubectl            /usr/bin/kubectl

ENV NVIDIA_DISABLE_REQUIRE="true"
ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_MIG_CONFIG_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=utility

ARG VERSION="N/A"
LABEL version=${VERSION}
LABEL release="N/A"
LABEL vendor="NVIDIA"
LABEL io.k8s.display-name="NVIDIA MIG Manager for Kubernetes"
LABEL name="NVIDIA MIG Manager for Kubernetes"
LABEL summary="NVIDIA MIG Manager for Kubernetes"
LABEL description="See summary"

ENTRYPOINT ["nvidia-mig-manager"]
