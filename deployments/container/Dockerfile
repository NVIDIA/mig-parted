# Copyright (c) 2021-2022, NVIDIA CORPORATION.  All rights reserved.
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

FROM nvcr.io/nvidia/cuda:13.0.1-base-ubi9 as build

SHELL ["/bin/bash", "-c"]

RUN yum install -y \
    wget make git gcc \
     && \
    rm -rf /var/cache/yum/*

ARG GOLANG_VERSION=1.22.8
RUN set -eux; \
    \
    arch="$(uname -m)"; \
    case "${arch##*-}" in \
        x86_64 | amd64) ARCH='amd64' ;; \
        ppc64el | ppc64le) ARCH='ppc64le' ;; \
        aarch64 | arm64) ARCH='arm64' ;; \
        *) echo "unsupported architecture" ; exit 1 ;; \
    esac; \
    wget -nv -O - https://storage.googleapis.com/golang/go${GOLANG_VERSION}.linux-${ARCH}.tar.gz \
    | tar -C /usr/local -xz

ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

ARG GOPROXY="https://proxy.golang.org,direct"
ENV GOPROXY=$GOPROXY

WORKDIR /build
COPY . .

RUN mkdir /artifacts
ARG VERSION="N/A"
ARG GIT_COMMIT="unknown"
RUN make PREFIX=/artifacts cmds

# Install the nvidia-ctk binary as a go executable
# TODO: Once we can consume a golang API from reconfigure.sh we can remove this.
ARG NVIDIA_CTK_VERSION=main
RUN go install -ldflags "-extldflags=-Wl,-z,lazy -s -w" \
    github.com/NVIDIA/nvidia-container-toolkit/cmd/nvidia-ctk@${NVIDIA_CTK_VERSION} \
    && cp ${GOPATH}/bin/nvidia-ctk /artifacts/nvidia-ctk

FROM nvcr.io/nvidia/distroless/go:v3.1.13-dev

USER 0:0
SHELL ["/busybox/sh", "-c"]
RUN ln -s /busybox/sh /bin/sh

COPY --from=build /artifacts/nvidia-mig-parted  /usr/bin/nvidia-mig-parted
COPY --from=build /artifacts/nvidia-mig-manager /usr/bin/nvidia-mig-manager
COPY --from=build /artifacts/nvidia-ctk         /usr/bin/nvidia-ctk

ENV NVIDIA_DISABLE_REQUIRE="true"
ENV NVIDIA_VISIBLE_DEVICES=all
ENV NVIDIA_MIG_CONFIG_DEVICES=all
ENV NVIDIA_DRIVER_CAPABILITIES=utility

LABEL version="${VERSION}"
LABEL release="N/A"
LABEL vendor="NVIDIA"
LABEL io.k8s.display-name="NVIDIA MIG Manager for Kubernetes"
LABEL name="NVIDIA MIG Manager for Kubernetes"
LABEL summary="NVIDIA MIG Manager for Kubernetes"
LABEL description="See summary"

ENTRYPOINT ["nvidia-mig-manager"]
