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

# benchmark-perf.sh — Benchmark nvidia-mig-parted performance.
#
# Captures wall time, CPU time (user + sys), peak RSS, and dynamic
# linker activity (dlsym/dlopen/dlclose counts via ltrace) across
# five scenarios:
#
#   1. assert --mode-only               Individual command
#   2. apply --mode-only --skip-reset   Individual command
#   3. apply                            Individual command
#   4. export                           Individual command
#   5. service.sh (full sequence)       Replicates the exact command
#                                       sequence from
#                                       deployments/systemd/service.sh:
#                                       assert → conditional apply
#                                       --mode-only → apply → export.
#                                       This directly maps to the CPU
#                                       time seen during
#                                       systemctl start nvidia-mig-manager.
#
# The dlsym count is the key correlating metric: each redundant
# nvml.Init()/Shutdown() cycle triggers a dlopen of libnvidia-ml.so.1
# followed by ~24 dlsym calls to resolve versioned API symbols, then
# a dlclose. Eliminating redundant cycles reduces dlsym calls
# proportionally to the CPU time improvement.
#
# Each scenario runs N times (default 3) for CPU timing. The ltrace
# pass runs once per scenario (dlsym counts are deterministic) and
# has a timeout to avoid hangs with some Go runtime/ltrace versions.
#
# Prerequisites (on target GPU system):
#   - nvidia-smi, GNU time (/usr/bin/time)
#   - ltrace (optional, for dlsym/dlopen/dlclose counts)
#
# Usage:
#   ./hack/benchmark-perf.sh <binary> <config-file> <config-label> [runs]
#
# Example:
#   ./hack/benchmark-perf.sh ./nvidia-mig-parted ./examples/config.yaml all-disabled
#   ./hack/benchmark-perf.sh ./nvidia-mig-parted ./examples/config.yaml all-disabled 5

set -euo pipefail

BINARY="${1:?Usage: $0 <binary> <config-file> <config-label> [runs]}"
CONFIG_FILE="${2:?Usage: $0 <binary> <config-file> <config-label> [runs]}"
CONFIG_LABEL="${3:?Usage: $0 <binary> <config-file> <config-label> [runs]}"
NUM_RUNS="${4:-3}"

LTRACE_TIMEOUT=30  # seconds; kill ltrace if it hangs

# ── Helpers ──────────────────────────────────────────────────────────

die()  { echo "ERROR: $*" >&2; exit 1; }
info() { echo ">>> $*"; }

check_prereqs() {
    [[ -x "${BINARY}" ]]        || die "Binary not found or not executable: ${BINARY}"
    [[ -f "${CONFIG_FILE}" ]]   || die "Config file not found: ${CONFIG_FILE}"
    command -v nvidia-smi >/dev/null 2>&1 || die "nvidia-smi not found"
    command -v ltrace >/dev/null 2>&1     || HAVE_LTRACE=false
    command -v jq >/dev/null 2>&1         || HAVE_JQ=false

    # GNU time is required for CPU time + RSS capture.
    # On most Linux systems this is /usr/bin/time.
    if /usr/bin/time --version 2>&1 | grep -q GNU; then
        GNU_TIME="/usr/bin/time"
    elif command -v gtime >/dev/null 2>&1; then
        GNU_TIME="gtime"
    else
        die "GNU time not found (/usr/bin/time or gtime). Install with: apt install time"
    fi
}

gpu_info() {
    local count name
    count=$(nvidia-smi --query-gpu=count --format=csv,noheader,nounits | head -1)
    name=$(nvidia-smi --query-gpu=name --format=csv,noheader | head -1)
    echo "${count}x ${name}"
}

# Run a single command, capture timing metrics.
# Sets: _wall _user _sys _rss _exit_code
run_once() {
    local cmd=("$@")
    local timefile

    timefile=$(mktemp /tmp/bench-time.XXXXXX)

    # Capture wall/user/sys/RSS via GNU time.
    # GNU time prints "Command exited with non-zero status N" to its
    # output file when the command fails, so we use a unique format
    # prefix "METRICS:" and grep for it to extract only the numbers.
    _exit_code=0
    ${GNU_TIME} -f "METRICS: %e %U %S %M" -o "${timefile}" \
        "${cmd[@]}" >/dev/null 2>/dev/null || _exit_code=$?

    local metrics_line
    metrics_line=$(grep '^METRICS:' "${timefile}" 2>/dev/null || true)
    if [[ -n "${metrics_line}" ]]; then
        read -r _ _wall _user _sys _rss <<< "${metrics_line}"
    else
        _wall=0; _user=0; _sys=0; _rss=0
    fi

    rm -f "${timefile}"
}

# Run a single ltrace pass for a command (with timeout).
# Sets: _dlsym _dlopen _dlclose
# dlsym counts are deterministic, so this only needs to run once
# per scenario. Uses a timeout to avoid hangs with some Go
# runtime / ltrace version combinations.
run_ltrace() {
    local cmd=("$@")
    local ltracefile

    ltracefile=$(mktemp /tmp/bench-ltrace.XXXXXX)

    _dlsym="-"; _dlopen="-"; _dlclose="-"

    # Run ltrace with a timeout. ltrace 0.7.3 can deadlock on
    # heavily-threaded Go binaries, so we kill it if it hangs.
    timeout "${LTRACE_TIMEOUT}" \
        ltrace -e dlsym+dlopen+dlclose -c -f -o "${ltracefile}" \
        "${cmd[@]}" >/dev/null 2>/dev/null || true

    # ltrace -c outputs a summary table like:
    #   % time     seconds  usecs/call     calls      function
    #   ------ ----------- ----------- --------- --------------------
    #    87.59    1.212403        2119       572 dlsym
    # Extract the 'calls' column for each function.
    _dlsym=$(awk '$NF == "dlsym" {print $4}' "${ltracefile}" 2>/dev/null) || _dlsym=0
    [[ -z "${_dlsym}" ]] && _dlsym=0
    _dlopen=$(awk '$NF == "dlopen" {print $4}' "${ltracefile}" 2>/dev/null) || _dlopen=0
    [[ -z "${_dlopen}" ]] && _dlopen=0
    _dlclose=$(awk '$NF == "dlclose" {print $4}' "${ltracefile}" 2>/dev/null) || _dlclose=0
    [[ -z "${_dlclose}" ]] && _dlclose=0

    rm -f "${ltracefile}"
}

# ── service.sh sequence wrapper ──────────────────────────────────────
#
# Replicates the exact nvidia-mig-parted invocation sequence from
# deployments/systemd/service.sh (happy path: mode already applied).
# This is what systemctl start nvidia-mig-manager actually runs.

write_service_script() {
    local script="$1"
    cat > "${script}" <<EOFSCRIPT
#!/usr/bin/env bash
set -e
BINARY="\$1"
CONFIG_FILE="\$2"
CONFIG_LABEL="\$3"

# Step 1: assert --mode-only (check if desired mode is applied)
\${BINARY} assert --mode-only -f "\${CONFIG_FILE}" -c "\${CONFIG_LABEL}" || {
    # If not applied, go through the apply flow
    \${BINARY} apply --mode-only --skip-reset -f "\${CONFIG_FILE}" -c "\${CONFIG_LABEL}"
    \${BINARY} apply --mode-only -f "\${CONFIG_FILE}" -c "\${CONFIG_LABEL}"
}

# Step 2: apply full config
\${BINARY} apply -f "\${CONFIG_FILE}" -c "\${CONFIG_LABEL}"

# Step 3: export
\${BINARY} export
EOFSCRIPT
    chmod +x "${script}"
}

# ── Scenario definitions ─────────────────────────────────────────────

declare -a SCENARIO_NAMES=(
    "assert --mode-only"
    "apply --mode-only --skip-reset"
    "apply"
    "export"
    "service.sh (full sequence)"
)

SERVICE_SCRIPT=$(mktemp /tmp/bench-service.XXXXXX)
write_service_script "${SERVICE_SCRIPT}"
trap "rm -f ${SERVICE_SCRIPT}" EXIT

scenario_cmd() {
    local idx=$1
    case ${idx} in
        0) echo "${BINARY} assert --mode-only -f ${CONFIG_FILE} -c ${CONFIG_LABEL}" ;;
        1) echo "${BINARY} apply --mode-only --skip-reset -f ${CONFIG_FILE} -c ${CONFIG_LABEL}" ;;
        2) echo "${BINARY} apply -f ${CONFIG_FILE} -c ${CONFIG_LABEL}" ;;
        3) echo "${BINARY} export" ;;
        4) echo "${SERVICE_SCRIPT} ${BINARY} ${CONFIG_FILE} ${CONFIG_LABEL}" ;;
    esac
}

# ── Arithmetic helpers (pure bash, no bc dependency) ─────────────────

# Add two decimal numbers: add "1.23" "4.56" → "5.79"
add() {
    awk "BEGIN {printf \"%.4f\", $1 + $2}"
}

# Divide: divide "12.34" "3" → "4.1133"
divide() {
    awk "BEGIN {printf \"%.4f\", $1 / $2}"
}

# Min of two decimals
minf() {
    awk "BEGIN {print ($1 < $2) ? $1 : $2}"
}

# Max of two decimals
maxf() {
    awk "BEGIN {print ($1 > $2) ? $1 : $2}"
}

# ── Main ─────────────────────────────────────────────────────────────

HAVE_LTRACE=true
HAVE_JQ=true
check_prereqs

GPU_INFO=$(gpu_info)
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
RESULT_FILE="benchmark-results-${TIMESTAMP}.json"

info "nvidia-mig-parted performance benchmark"
info "Binary:     ${BINARY}"
info "Config:     ${CONFIG_FILE} (${CONFIG_LABEL})"
info "GPUs:       ${GPU_INFO}"
info "Runs:       ${NUM_RUNS} per scenario"
info "ltrace:     ${HAVE_LTRACE}"
info ""

# Associative arrays keyed by scenario index.
declare -A sum_wall sum_user sum_sys sum_rss
declare -A min_wall max_wall min_user max_user min_sys max_sys
declare -A min_rss max_rss
declare -A dl_sym dl_open dl_close

NUM_SCENARIOS=${#SCENARIO_NAMES[@]}

for (( s=0; s<NUM_SCENARIOS; s++ )); do
    sum_wall[$s]=0; sum_user[$s]=0; sum_sys[$s]=0; sum_rss[$s]=0
    min_wall[$s]=999999; max_wall[$s]=0
    min_user[$s]=999999; max_user[$s]=0
    min_sys[$s]=999999;  max_sys[$s]=0
    min_rss[$s]=999999;  max_rss[$s]=0
    dl_sym[$s]="-"; dl_open[$s]="-"; dl_close[$s]="-"
done

# ── Phase 1: CPU timing (N runs per scenario) ────────────────────────

for (( r=1; r<=NUM_RUNS; r++ )); do
    info "Run ${r}/${NUM_RUNS}"

    for (( s=0; s<NUM_SCENARIOS; s++ )); do
        cmd_str=$(scenario_cmd $s)
        read -r -a cmd_arr <<< "${cmd_str}"
        printf "  %-35s" "${SCENARIO_NAMES[$s]}"

        run_once "${cmd_arr[@]}"

        exit_info=""
        [[ "${_exit_code}" != 0 ]] && exit_info=" (exit=${_exit_code})"
        printf "wall=%s user=%s sys=%s rss=%s%s\n" \
            "${_wall}" "${_user}" "${_sys}" "${_rss}" "${exit_info}"

        sum_wall[$s]=$(add "${sum_wall[$s]}" "${_wall}")
        sum_user[$s]=$(add "${sum_user[$s]}" "${_user}")
        sum_sys[$s]=$(add "${sum_sys[$s]}" "${_sys}")
        sum_rss[$s]=$(add "${sum_rss[$s]}" "${_rss}")

        min_wall[$s]=$(minf "${min_wall[$s]}" "${_wall}")
        max_wall[$s]=$(maxf "${max_wall[$s]}" "${_wall}")
        min_user[$s]=$(minf "${min_user[$s]}" "${_user}")
        max_user[$s]=$(maxf "${max_user[$s]}" "${_user}")
        min_sys[$s]=$(minf "${min_sys[$s]}" "${_sys}")
        max_sys[$s]=$(maxf "${max_sys[$s]}" "${_sys}")
        min_rss[$s]=$(minf "${min_rss[$s]}" "${_rss}")
        max_rss[$s]=$(maxf "${max_rss[$s]}" "${_rss}")
    done
    echo
done

# ── Phase 2: ltrace pass (once per scenario, with timeout) ───────────

if [[ "${HAVE_LTRACE}" != false ]]; then
    info "ltrace pass (once per scenario, ${LTRACE_TIMEOUT}s timeout)"

    for (( s=0; s<NUM_SCENARIOS; s++ )); do
        cmd_str=$(scenario_cmd $s)
        read -r -a cmd_arr <<< "${cmd_str}"
        printf "  %-35s" "${SCENARIO_NAMES[$s]}"

        run_ltrace "${cmd_arr[@]}"

        dl_sym[$s]="${_dlsym}"
        dl_open[$s]="${_dlopen}"
        dl_close[$s]="${_dlclose}"

        printf "dlsym=%s dlopen=%s dlclose=%s\n" \
            "${_dlsym}" "${_dlopen}" "${_dlclose}"
    done
    echo
fi

# ── Summary table ────────────────────────────────────────────────────

echo "========================================"
echo "nvidia-mig-parted performance benchmark"
echo "========================================"
echo "Binary:  ${BINARY}"
echo "Config:  ${CONFIG_FILE} (${CONFIG_LABEL})"
echo "GPUs:    ${GPU_INFO}"
echo "Runs:    ${NUM_RUNS} per scenario"
echo "Date:    $(date +%Y-%m-%d)"
echo ""

if [[ "${HAVE_LTRACE}" != false ]]; then
    printf "%-35s  %8s  %8s  %8s  %8s  %8s  %6s  %6s  %7s\n" \
        "Scenario" "Wall(s)" "User(s)" "Sys(s)" "CPU(s)" "RSS(KB)" "dlsym" "dlopen" "dlclose"
    printf "%-35s  %8s  %8s  %8s  %8s  %8s  %6s  %6s  %7s\n" \
        "---" "---" "---" "---" "---" "---" "---" "---" "---"
else
    printf "%-35s  %8s  %8s  %8s  %8s  %8s\n" \
        "Scenario" "Wall(s)" "User(s)" "Sys(s)" "CPU(s)" "RSS(KB)"
    printf "%-35s  %8s  %8s  %8s  %8s  %8s\n" \
        "---" "---" "---" "---" "---" "---"
fi

for (( s=0; s<NUM_SCENARIOS; s++ )); do
    avg_wall=$(divide "${sum_wall[$s]}" "${NUM_RUNS}")
    avg_user=$(divide "${sum_user[$s]}" "${NUM_RUNS}")
    avg_sys=$(divide "${sum_sys[$s]}" "${NUM_RUNS}")
    avg_cpu=$(add "${avg_user}" "${avg_sys}")
    avg_rss=$(divide "${sum_rss[$s]}" "${NUM_RUNS}")

    if [[ "${HAVE_LTRACE}" != false ]]; then
        printf "%-35s  %8.2f  %8.2f  %8.2f  %8.2f  %8.0f  %6s  %6s  %7s\n" \
            "${SCENARIO_NAMES[$s]}" \
            "${avg_wall}" "${avg_user}" "${avg_sys}" "${avg_cpu}" "${avg_rss}" \
            "${dl_sym[$s]}" "${dl_open[$s]}" "${dl_close[$s]}"
    else
        printf "%-35s  %8.2f  %8.2f  %8.2f  %8.2f  %8.0f\n" \
            "${SCENARIO_NAMES[$s]}" \
            "${avg_wall}" "${avg_user}" "${avg_sys}" "${avg_cpu}" "${avg_rss}"
    fi
done

echo ""
echo "(CPU values are averages across ${NUM_RUNS} runs; dlsym/dlopen/dlclose are from a single ltrace pass)"

# ── JSON output ──────────────────────────────────────────────────────

json_scenarios=""
for (( s=0; s<NUM_SCENARIOS; s++ )); do
    avg_wall=$(divide "${sum_wall[$s]}" "${NUM_RUNS}")
    avg_user=$(divide "${sum_user[$s]}" "${NUM_RUNS}")
    avg_sys=$(divide "${sum_sys[$s]}" "${NUM_RUNS}")
    avg_rss=$(divide "${sum_rss[$s]}" "${NUM_RUNS}")

    if [[ "${HAVE_LTRACE}" != false ]]; then
        ltrace_json=",
      \"dlsym\":        ${dl_sym[$s]},
      \"dlopen\":       ${dl_open[$s]},
      \"dlclose\":      ${dl_close[$s]}"
    else
        ltrace_json=""
    fi

    [[ -n "${json_scenarios}" ]] && json_scenarios+=","
    json_scenarios+="
    {
      \"name\": \"${SCENARIO_NAMES[$s]}\",
      \"wall_s\":       {\"avg\": ${avg_wall}, \"min\": ${min_wall[$s]}, \"max\": ${max_wall[$s]}},
      \"user_cpu_s\":   {\"avg\": ${avg_user}, \"min\": ${min_user[$s]}, \"max\": ${max_user[$s]}},
      \"sys_cpu_s\":    {\"avg\": ${avg_sys},  \"min\": ${min_sys[$s]},  \"max\": ${max_sys[$s]}},
      \"rss_kb\":       {\"avg\": ${avg_rss},  \"min\": ${min_rss[$s]},  \"max\": ${max_rss[$s]}}${ltrace_json}
    }"
done

cat > "${RESULT_FILE}" <<ENDJSON
{
  "timestamp": "${TIMESTAMP}",
  "binary": "${BINARY}",
  "config_file": "${CONFIG_FILE}",
  "config_label": "${CONFIG_LABEL}",
  "gpus": "${GPU_INFO}",
  "num_runs": ${NUM_RUNS},
  "scenarios": [${json_scenarios}
  ]
}
ENDJSON

info "Raw results saved to ${RESULT_FILE}"
