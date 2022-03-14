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
	"fmt"
	"sort"

	"github.com/NVIDIA/mig-parted/pkg/types"
)

const (
	A100_SXM4_40GB types.DeviceID = 0x20B010DE
)

type LoopControl int

const (
	Continue LoopControl = iota
	Break
)

const (
	mig_1c_1g_5gb    = types.MigProfile("1c.1g.5gb")
	mig_1c_1g_5gb_me = types.MigProfile("1c.1g.5gb+me")
	mig_1c_2g_10gb   = types.MigProfile("1c.2g.10gb")
	mig_2c_2g_10gb   = types.MigProfile("2c.2g.10gb")
	mig_1c_3g_20gb   = types.MigProfile("1c.3g.20gb")
	mig_2c_3g_20gb   = types.MigProfile("2c.3g.20gb")
	mig_3c_3g_20gb   = types.MigProfile("3c.3g.20gb")
	mig_1c_4g_20gb   = types.MigProfile("1c.4g.20gb")
	mig_2c_4g_20gb   = types.MigProfile("2c.4g.20gb")
	mig_4c_4g_20gb   = types.MigProfile("4c.4g.20gb")
	mig_1c_7g_40gb   = types.MigProfile("1c.7g.40gb")
	mig_2c_7g_40gb   = types.MigProfile("2c.7g.40gb")
	mig_3c_7g_40gb   = types.MigProfile("3c.7g.40gb")
	mig_4c_7g_40gb   = types.MigProfile("4c.7g.40gb")
	mig_7c_7g_40gb   = types.MigProfile("7c.7g.40gb")
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

var a100_sxm4_40gb_config_group a100_sxm4_40gb_MigConfigGroup

func NewA100_SXM4_40GB_MigConfigGroup() types.MigConfigGroup {
	if a100_sxm4_40gb_config_group.Configs == nil {
		a100_sxm4_40gb_config_group.init()
	}
	return &a100_sxm4_40gb_config_group
}

func (m *a100_sxm4_40gb_MigConfigGroup) init() {
	configs := make(map[string]types.MigConfig)
	m.iterateDeviceTypes(func(mps []types.MigProfile) LoopControl {
		cis := 0
		cis_per_gi := make(map[int]int)
		mes_per_gi := make(map[int]int)
		for _, mp := range mps {
			c, g, _, _ := mp.MustParse()
			cis += c
			cis_per_gi[g] += c
			if mp.HasAttribute(types.AttributeMediaExtensions) {
				mes_per_gi[g]++
			}
		}

		if cis_per_gi[1] > 1 && mes_per_gi[1] > 0 {
			return Break
		}

		if cis > 7 {
			return Break
		}

		unique_gis := 0
		for gi := range cis_per_gi {
			unique_gis += gi
		}

		if unique_gis > 7 {
			return Break
		}

		sort.Slice(mps, func(i, j int) bool {
			return mps[i] < mps[j]
		})

		str := fmt.Sprintf("%v", mps)
		if _, exists := configs[str]; !exists {
			configs[str] = types.NewMigConfig(mps)
		}

		if cis < 7 {
			return Continue
		}

		return Break
	})
	for _, v := range configs {
		m.Configs = append(m.Configs, v)
	}
}

func (m *a100_sxm4_40gb_MigConfigGroup) GetDeviceTypes() []types.MigProfile {
	return []types.MigProfile{
		mig_1c_1g_5gb,
		mig_1c_1g_5gb_me,
		mig_1c_2g_10gb,
		mig_2c_2g_10gb,
		mig_1c_3g_20gb,
		mig_2c_3g_20gb,
		mig_3c_3g_20gb,
		mig_1c_4g_20gb,
		mig_2c_4g_20gb,
		mig_4c_4g_20gb,
		mig_1c_7g_40gb,
		mig_2c_7g_40gb,
		mig_3c_7g_40gb,
		mig_4c_7g_40gb,
		mig_7c_7g_40gb,
	}
}

func (m *a100_sxm4_40gb_MigConfigGroup) iterateDeviceTypes(f func([]types.MigProfile) LoopControl) {
	maxDevices := types.MigConfig{
		mig_1c_1g_5gb:    7,
		mig_1c_1g_5gb_me: 1,
		mig_1c_2g_10gb:   6,
		mig_2c_2g_10gb:   3,
		mig_1c_3g_20gb:   6,
		mig_2c_3g_20gb:   2,
		mig_3c_3g_20gb:   2,
		mig_1c_4g_20gb:   4,
		mig_2c_4g_20gb:   2,
		mig_4c_4g_20gb:   1,
		mig_1c_7g_40gb:   7,
		mig_2c_7g_40gb:   3,
		mig_3c_7g_40gb:   2,
		mig_4c_7g_40gb:   1,
		mig_7c_7g_40gb:   1,
	}.Flatten()

	var iterate func(i int, accum []types.MigProfile) LoopControl
	iterate = func(i int, accum []types.MigProfile) LoopControl {
		accum = append(accum, maxDevices[i])
		control := f(accum)
		if control == Break {
			return Continue
		}
		for j := i + 1; j < len(maxDevices); j++ {
			iterate(j, accum)
		}
		return Continue
	}

	for i := 0; i < len(maxDevices); i++ {
		iterate(i, []types.MigProfile{})
	}
}
