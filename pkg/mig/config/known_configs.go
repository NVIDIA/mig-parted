/*
 * Copyright (c) 2021, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"github.com/NVIDIA/mig-parted/pkg/types"
)

const (
	A100_SXM4_40GB types.DeviceID = 0x20B010DE
)

const (
	mig_1g_5gb  = types.MigProfile("1g.5gb")
	mig_2g_10gb = types.MigProfile("2g.10gb")
	mig_3g_20gb = types.MigProfile("3g.20gb")
	mig_4g_20gb = types.MigProfile("4g.20gb")
	mig_7g_40gb = types.MigProfile("7g.40gb")
)

func GetKnownMigConfigGroups() types.MigConfigGroups {
	return types.MigConfigGroups{
		A100_SXM4_40GB: NewA100_SXM4_40GB_MigConfigGroup(),
	}
}

// A100_SXM4_40GB
type a100_sxm4_40gb_MigConfigGroup struct {
	types.MigConfigGroupBase
}

func NewA100_SXM4_40GB_MigConfigGroup() types.MigConfigGroup {
	configs := []types.MigConfig{
		{
			mig_1g_5gb:  1,
			mig_2g_10gb: 1,
			mig_3g_20gb: 0,
			mig_4g_20gb: 1,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  3,
			mig_2g_10gb: 0,
			mig_3g_20gb: 0,
			mig_4g_20gb: 1,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  0,
			mig_2g_10gb: 0,
			mig_3g_20gb: 2,
			mig_4g_20gb: 0,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  1,
			mig_2g_10gb: 1,
			mig_3g_20gb: 1,
			mig_4g_20gb: 0,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  3,
			mig_2g_10gb: 0,
			mig_3g_20gb: 1,
			mig_4g_20gb: 0,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  0,
			mig_2g_10gb: 2,
			mig_3g_20gb: 1,
			mig_4g_20gb: 0,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  1,
			mig_2g_10gb: 3,
			mig_3g_20gb: 0,
			mig_4g_20gb: 0,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  3,
			mig_2g_10gb: 2,
			mig_3g_20gb: 0,
			mig_4g_20gb: 0,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  2,
			mig_2g_10gb: 1,
			mig_3g_20gb: 1,
			mig_4g_20gb: 0,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  5,
			mig_2g_10gb: 1,
			mig_3g_20gb: 0,
			mig_4g_20gb: 0,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  4,
			mig_2g_10gb: 0,
			mig_3g_20gb: 1,
			mig_4g_20gb: 0,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  7,
			mig_2g_10gb: 0,
			mig_3g_20gb: 0,
			mig_4g_20gb: 0,
			mig_7g_40gb: 0,
		},
		{
			mig_1g_5gb:  0,
			mig_2g_10gb: 0,
			mig_3g_20gb: 0,
			mig_4g_20gb: 0,
			mig_7g_40gb: 1,
		},
	}

	return &a100_sxm4_40gb_MigConfigGroup{
		types.MigConfigGroupBase{
			Configs: configs,
		},
	}
}

func (m *a100_sxm4_40gb_MigConfigGroup) GetDeviceTypes() []types.MigProfile {
	return []types.MigProfile{
		mig_1g_5gb,
		mig_2g_10gb,
		mig_3g_20gb,
		mig_4g_20gb,
		mig_7g_40gb,
	}
}
