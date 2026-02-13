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

SOURCE1 = nvidia-mig-parted
SOURCE2 = nvidia-mig-manager.service
SOURCE3 = nvidia-mig-parted.sh
SOURCE4 = override.conf
SOURCE5 = service.sh
SOURCE6 = utils.sh
SOURCE7 = hooks.sh
SOURCE8 = hooks-default.yaml
SOURCE9 = hooks-minimal.yaml
SOURCE10 = config-default.yaml
SOURCE11 = nvidia-gpu-reset.target

build:

install:
	mkdir -p $(DESTDIR)/usr/bin
	mkdir -p $(DESTDIR)/lib/systemd/system
	mkdir -p $(DESTDIR)/etc/profile.d
	mkdir -p $(DESTDIR)/etc/systemd/system/nvidia-mig-manager.service.d
	mkdir -p $(DESTDIR)/etc/nvidia-mig-manager
	mkdir -p $(DESTDIR)/var/lib/nvidia-mig-manager
	
	install -m 755 -t $(DESTDIR)/usr/bin $(SOURCE1)
	install -m 644 -t $(DESTDIR)/lib/systemd/system $(SOURCE2)
	install -m 644 -t $(DESTDIR)/etc/profile.d $(SOURCE3)
	install -m 644 -t $(DESTDIR)/etc/systemd/system/nvidia-mig-manager.service.d $(SOURCE4)
	install -m 755 -t $(DESTDIR)/etc/nvidia-mig-manager $(SOURCE5)
	install -m 644 -t $(DESTDIR)/etc/nvidia-mig-manager $(SOURCE6)
	install -m 644 -t $(DESTDIR)/etc/nvidia-mig-manager $(SOURCE7)
	install -m 644 -t $(DESTDIR)/etc/nvidia-mig-manager $(SOURCE8)
	install -m 644 -t $(DESTDIR)/etc/nvidia-mig-manager $(SOURCE9)
	install -m 644 -t $(DESTDIR)/etc/nvidia-mig-manager $(SOURCE10)
	install -m 644 -t $(DESTDIR)/lib/systemd/system $(SOURCE11)
