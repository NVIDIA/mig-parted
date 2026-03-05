#!/usr/bin/env bash

# Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
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

# benchmark-compare.sh — Compare two benchmark result JSON files.
#
# Reads the JSON files produced by benchmark-perf.sh and prints a
# side-by-side comparison with deltas and percentage improvements
# for CPU time, wall time, and dynamic linker call counts.
#
# Usage:
#   ./hack/benchmark-compare.sh <before.json> <after.json>
#
# Example:
#   ./hack/benchmark-compare.sh benchmark-results-20260303-111337.json benchmark-results-20260303-111809.json

set -euo pipefail

BEFORE="${1:?Usage: $0 <before.json> <after.json>}"
AFTER="${2:?Usage: $0 <before.json> <after.json>}"

[[ -f "${BEFORE}" ]] || { echo "ERROR: File not found: ${BEFORE}" >&2; exit 1; }
[[ -f "${AFTER}" ]]  || { echo "ERROR: File not found: ${AFTER}" >&2; exit 1; }

command -v python3 >/dev/null 2>&1 || { echo "ERROR: python3 is required" >&2; exit 1; }

python3 - "${BEFORE}" "${AFTER}" <<'PYEOF'
import json
import sys

def load(path):
    with open(path) as f:
        return json.load(f)

before = load(sys.argv[1])
after = load(sys.argv[2])

before_map = {s["name"]: s for s in before["scenarios"]}
after_map = {s["name"]: s for s in after["scenarios"]}

def avg(scenario, key):
    v = scenario.get(key)
    if v is None:
        return None
    if isinstance(v, dict):
        return v.get("avg")
    return v

def fmt_delta(old, new):
    if old is None or new is None:
        return "-", "-"
    delta = new - old
    pct = "-" if old == 0 else f"{(delta / old) * 100:+.1f}%"
    return f"{delta:+.2f}", pct

def fmt_delta_int(old, new):
    if old is None or new is None:
        return "-", "-"
    delta = new - old
    if old == 0:
        pct = "0.0%" if new == 0 else "-"
    else:
        pct = f"{(delta / old) * 100:+.1f}%"
    return f"{delta:+d}", pct

def speedup(old, new):
    if old is None or new is None or new == 0:
        return "-"
    return f"{old / new:.1f}x"

def row(name, b_str, a_str, delta, pct, spd):
    print(f"  {name:<33s}  {b_str:>8s}  {a_str:>8s}  {delta:>8s}  {pct:>8s}  {spd:>7s}")

# Header
print("=" * 90)
print("nvidia-mig-parted benchmark comparison")
print("=" * 90)
print(f"Before:  {sys.argv[1]}")
print(f"  Binary:  {before.get('binary', '?')}")
print(f"  GPUs:    {before.get('gpus', '?')}")
print(f"After:   {sys.argv[2]}")
print(f"  Binary:  {after.get('binary', '?')}")
print(f"  GPUs:    {after.get('gpus', '?')}")
print()

# Table header
print("-" * 90)
print(f"  {'Scenario':<33s}  {'Before':>8s}  {'After':>8s}  {'Delta':>8s}  {'Change':>8s}  {'Speedup':>7s}")
print("-" * 90)

# CPU time
print("CPU time (user + sys, seconds):")
for name in before_map:
    if name not in after_map:
        continue
    b, a = before_map[name], after_map[name]
    b_user, b_sys = avg(b, "user_cpu_s"), avg(b, "sys_cpu_s")
    a_user, a_sys = avg(a, "user_cpu_s"), avg(a, "sys_cpu_s")
    b_cpu = b_user + b_sys if None not in (b_user, b_sys) else None
    a_cpu = a_user + a_sys if None not in (a_user, a_sys) else None
    delta, pct = fmt_delta(b_cpu, a_cpu)
    row(name,
        f"{b_cpu:.2f}" if b_cpu is not None else "-",
        f"{a_cpu:.2f}" if a_cpu is not None else "-",
        delta, pct, speedup(b_cpu, a_cpu))

# Wall time
print()
print("Wall time (seconds):")
for name in before_map:
    if name not in after_map:
        continue
    b, a = before_map[name], after_map[name]
    b_wall, a_wall = avg(b, "wall_s"), avg(a, "wall_s")
    delta, pct = fmt_delta(b_wall, a_wall)
    row(name,
        f"{b_wall:.2f}" if b_wall is not None else "-",
        f"{a_wall:.2f}" if a_wall is not None else "-",
        delta, pct, speedup(b_wall, a_wall))

# dlsym/dlopen/dlclose (if available)
if any("dlsym" in s for s in before["scenarios"]):
    for dl_key, dl_label in [("dlsym", "dlsym calls"), ("dlopen", "dlopen calls"), ("dlclose", "dlclose calls")]:
        print()
        print(f"{dl_label}:")
        for name in before_map:
            if name not in after_map:
                continue
            b_val = before_map[name].get(dl_key)
            a_val = after_map[name].get(dl_key)
            if b_val is None and a_val is None:
                continue
            if isinstance(b_val, dict):
                b_val = b_val.get("avg")
            if isinstance(a_val, dict):
                a_val = a_val.get("avg")
            if b_val is not None:
                b_val = int(b_val)
            if a_val is not None:
                a_val = int(a_val)
            delta, pct = fmt_delta_int(b_val, a_val)
            row(name,
                str(b_val) if b_val is not None else "-",
                str(a_val) if a_val is not None else "-",
                delta, pct, speedup(b_val, a_val))

print()
print("-" * 90)
PYEOF
