#!/usr/bin/env bash
# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
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

set -euo pipefail

OUTPUT="${OUTPUT:-THIRD_PARTY_NOTICES.md}"
LICENSES_DIR="${LICENSES_DIR:-.licenses-cache}"
TOOLS_DIR="${TOOLS_DIR:-deployments/devel}"
TOOLS_FILE="${TOOLS_DIR}/tools.go"
MULTI_ARCH_MK="${MULTI_ARCH_MK:-deployments/container/multi-arch.mk}"
MODULES_TXT="${MODULES_TXT:-vendor/modules.txt}"

PACKAGES=("./cmd/...")

PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
)

die() {
    printf 'ERROR: %s\n' "$1" >&2
    shift
    if (( $# > 0 )); then
        printf '%s\n' "$@" >&2
    fi
    exit 1
}

log() {
    printf '%s\n' "$*" >&2
}

# Licenses that are themselves Markdown close a fixed ``` fence early and invert
# every block after it, so open with one backtick more than the file's longest run.
fence_for() {
    local file="$1" longest width
    # -a: a license containing a NUL byte is otherwise reported as "Binary file
    # ... matches" — on stdout or stderr by grep version, so the width varies by host.
    longest=$(LC_ALL=C grep -oaE '`+' "${file}" 2>/dev/null \
        | awk '{ if (length($0) > m) m = length($0) } END { print m+0 }')
    width=$(( longest + 1 ))
    (( width < 3 )) && width=3
    printf '%*s' "${width}" '' | tr ' ' '`'
}

check_prerequisites() {
    command -v go >/dev/null 2>&1 || die "go is not installed."

    # Absolute: the bundled pass chdirs.
    if [[ -x "./bin/go-licenses" ]]; then
        GO_LICENSES="${PWD}/bin/go-licenses"
    elif command -v go-licenses >/dev/null 2>&1; then
        GO_LICENSES="$(command -v go-licenses)"
    else
        die "go-licenses is not installed." "Install it with 'make third-party-notices'."
    fi

    local f
    for f in "${TOOLS_FILE}" "${MULTI_ARCH_MK}" "${MODULES_TXT}"; do
        [[ -f "${f}" ]] || die "${f} not found — run 'make third-party-notices' from the repo root."
    done

    LOCAL_MODULE=$(go list -m 2>/dev/null || true)
    [[ -n "${LOCAL_MODULE}" ]] || die "could not determine local module path via 'go list -m'."

    # CGO off lets go-licenses cross-list without a C toolchain; the reported
    # import closure is unchanged.
    export GOFLAGS="-mod=vendor"
    export CGO_ENABLED=0
}

verify_platform_matrix() {
    local expected actual
    expected=$(sed -n 's/^DOCKER_BUILD_PLATFORM_OPTIONS[[:space:]]*?*=[[:space:]]*--platform=//p' \
        "${MULTI_ARCH_MK}" | tr ',' '\n' | sed '/^$/d' | LC_ALL=C sort -u)
    [[ -n "${expected}" ]] \
        || die "could not read DOCKER_BUILD_PLATFORM_OPTIONS from ${MULTI_ARCH_MK}."

    actual=$(printf '%s\n' "${PLATFORMS[@]}" | LC_ALL=C sort -u)
    [[ "${expected}" == "${actual}" ]] || die \
        "the PLATFORMS matrix is out of sync with ${MULTI_ARCH_MK}." \
        "Update the PLATFORMS array in hack/generate-third-party-notices.sh to match the released targets." \
        "  matrix (PLATFORMS): $(echo "${actual}" | paste -sd ' ' -)" \
        "  image platforms:    $(echo "${expected}" | paste -sd ' ' -)"
}

prepare_workspace() {
    # Guard the override: '', '/', '.' or '..' would make this fatal.
    case "${LICENSES_DIR}" in
        ""|"/"|"."|"..")
            die "refusing to 'rm -rf' unsafe LICENSES_DIR='${LICENSES_DIR}'."
            ;;
    esac
    rm -rf "${LICENSES_DIR}"
    mkdir -p "${LICENSES_DIR}" "${LICENSES_DIR}/.tools"

    # Explicit templates: macOS mktemp ignores TMPDIR without one.
    local t="${TMPDIR:-/tmp}/mig-parted-notices"
    SAVE_ROOT="$(mktemp -d "${t}.XXXXXX")"
    COMBINED_CSV="$(mktemp "${t}-csv.XXXXXX")"
    INDEX_FILE="$(mktemp "${t}-idx.XXXXXX")"
    TOOLS_CSV="$(mktemp "${t}-tools-csv.XXXXXX")"
    TOOLS_INDEX="$(mktemp "${t}-tools-idx.XXXXXX")"

    # Next to the destination, not under TMPDIR: the final mv must not cross
    # filesystems, where rename(2) degrades to copy-then-unlink.
    local out_dir
    out_dir="$(dirname "${OUTPUT}")"
    mkdir -p "${out_dir}"
    OUT_TMP="$(mktemp "${out_dir}/.$(basename "${OUTPUT}").XXXXXX")"
    trap 'rm -rf "${SAVE_ROOT}"; rm -f "${COMBINED_CSV}" "${INDEX_FILE}" "${TOOLS_CSV}" "${TOOLS_INDEX}" "${OUT_TMP}"' EXIT INT TERM
}

collect_runtime() {
    local platform goos goarch save_dir

    for platform in "${PLATFORMS[@]}"; do
        goos="${platform%/*}"
        goarch="${platform#*/}"
        log "Collecting licenses for ${goos}/${goarch}..."

        save_dir="${SAVE_ROOT}/${goos}_${goarch}"

        # Only the local module: --ignore matches raw string prefixes, not path
        # segments, so a stdlib list would add the bare token "go" and silently
        # drop golang.org/x/*, google.golang.org/* and gopkg.in/*.
        GOOS="${goos}" GOARCH="${goarch}" "${GO_LICENSES}" save "${PACKAGES[@]}" \
            --save_path="${save_dir}" \
            --force \
            --ignore="${LOCAL_MODULE}"

        GOOS="${goos}" GOARCH="${goarch}" "${GO_LICENSES}" csv "${PACKAGES[@]}" \
            --ignore="${LOCAL_MODULE}" \
            >> "${COMBINED_CSV}"

        merge_licenses "${save_dir}" "${LICENSES_DIR}"
    done
}

collect_bundled() {
    local platform goos goarch save_dir

    read_tool_packages

    ( cd "${TOOLS_DIR}" && GOFLAGS="-mod=readonly" go mod download ) >&2

    for platform in "${PLATFORMS[@]}"; do
        goos="${platform%/*}"
        goarch="${platform#*/}"
        log "Collecting bundled binary licenses for ${goos}/${goarch}..."

        save_dir="${SAVE_ROOT}/tools/${goos}_${goarch}"
        (
            cd "${TOOLS_DIR}"
            # shellcheck disable=SC2030  # subshell-local on purpose; the outer -mod=vendor stands.
            export GOFLAGS="-mod=readonly"
            # nvidia-ctk is installed by the Dockerfile with cgo at its default,
            # so resolve it the same way. With CGO_ENABLED=0 the build tags drop
            # every file in go-nvml/pkg/dl on linux and go-licenses reports a
            # narrower license root that does not cover it.
            export CGO_ENABLED=1
            GOOS="${goos}" GOARCH="${goarch}" "${GO_LICENSES}" save "${TOOL_PKGS[@]}" \
                --save_path="${save_dir}" \
                --force \
                --ignore="${LOCAL_MODULE}" >&2
            GOOS="${goos}" GOARCH="${goarch}" "${GO_LICENSES}" csv "${TOOL_PKGS[@]}" \
                --ignore="${LOCAL_MODULE}"
        ) >> "${TOOLS_CSV}"

        merge_licenses "${save_dir}" "${LICENSES_DIR}/.tools"
    done
}

# Tools that run at build time and are not copied into any released artifact.
# Their dependencies are not redistributed, so they are not attributed here.
# Anything else in tools.go is included: over-attributing is the safe direction,
# and a new entry should have to be argued out rather than silently dropped.
UNSHIPPED_TOOLS=(
    "github.com/matryer/moq"
    "github.com/google/go-licenses/v2"
)

# tools.go is build-tagged and imports main packages, so it cannot be listed as
# a package; read the pinned paths out of it as 'make install-tools' does.
read_tool_packages() {
    TOOL_PKGS=()
    local pkg skip
    while IFS= read -r pkg; do
        [[ -n "${pkg}" ]] || continue
        skip=no
        for t in "${UNSHIPPED_TOOLS[@]}"; do
            [[ "${pkg}" == "${t}" ]] && skip=yes && break
        done
        [[ "${skip}" == "no" ]] && TOOL_PKGS+=("${pkg}")
    done < <(LC_ALL=C grep -E '^[[:space:]]*_ "' "${TOOLS_FILE}" | sed 's/.*"\(.*\)".*/\1/')

    (( ${#TOOL_PKGS[@]} > 0 )) || die "every import in ${TOOLS_FILE} is listed in UNSHIPPED_TOOLS."
}

# Module cache files are 0444 and cp preserves that, so the next platform's copy
# would fail.
merge_licenses() {
    cp -R "$1/." "$2/"
    chmod -R u+w "$2"
}

# One row per package, joining licenses rather than picking one: go-licenses
# emits a row per recognized license, so key-only dedup would hide the second —
# and picks a different row under BSD and GNU sort. LC_ALL=C for byte order.
collapse_index() {
    LC_ALL=C sort -u "$1" | awk -F, '
        {
            pkg = $1
            if (!(pkg in url)) { url[pkg] = $2; order[++n] = pkg }
            if (!((pkg SUBSEP $3) in seen)) {
                seen[pkg SUBSEP $3] = 1
                # Count rather than test "pkg in lic": mawk and busybox awk
                # instantiate the assignment target before evaluating the RHS,
                # so that test is already true on the first row.
                lic[pkg] = (cnt[pkg]++ ? lic[pkg] " / " : "") $3
            }
        }
        END { for (i = 1; i <= n; i++) print order[i] "," url[order[i]] "," lic[order[i]] }
    '
}

# In vendor mode go-licenses reports a URL into this repo at HEAD, which stops
# describing released content once main moves; module@version is immutable.
annotate_modules() {
    awk -v modfile="${MODULES_TXT}" '
        BEGIN {
            FS = OFS = ","
            while ((getline line < modfile) > 0) {
                if (line !~ /^# /) continue
                split(line, f, " ")
                # "# <path> <version>", plus "=> <path> <version>" for a replace;
                # the replacement is what is vendored, and has no version if local.
                if (f[4] == "=>" || f[3] == "=>") {
                    r = (f[4] == "=>") ? 5 : 4
                    if (f[r + 1] == "") {
                        print "ERROR: " modfile " replaces " f[2] " with a local path;" > "/dev/stderr"
                        print "teach hack/generate-third-party-notices.sh how to attribute it." > "/dev/stderr"
                        exit 1
                    }
                    mods[++m] = f[2]
                    disp[f[2]] = f[r] "@" f[r + 1]
                } else {
                    mods[++m] = f[2]
                    disp[f[2]] = f[2] "@" f[3]
                }
            }
            close(modfile)
            # A read error makes getline return -1, labelling every entry "unknown".
            if (m == 0) {
                print "ERROR: no module lines read from " modfile > "/dev/stderr"
                exit 1
            }
        }
        {
            best = ""
            for (i = 1; i <= m; i++) {
                mp = mods[i]
                if (($1 == mp || index($1, mp "/") == 1) && length(mp) > length(best)) best = mp
            }
            print $0, (best == "" ? "unknown" : disp[best])
        }
    '
}

build_indexes() {
    log "Generating dependency index..."
    collapse_index "${COMBINED_CSV}" | annotate_modules > "${INDEX_FILE}"
    collapse_index "${TOOLS_CSV}" > "${TOOLS_INDEX}"

    [[ -s "${INDEX_FILE}" ]] \
        || die "go-licenses produced no entries for ${PACKAGES[*]} — refusing to write empty notices file."
    [[ -s "${TOOLS_INDEX}" ]] \
        || die "go-licenses produced no entries for the bundled binary — refusing to write incomplete notices file."

    # go-licenses reports a license it cannot classify as "Unknown" and still
    # exits 0. Anchored on both sides because licenses are joined.
    local idx
    for idx in "${INDEX_FILE}" "${TOOLS_INDEX}"; do
        if cut -d, -f3 "${idx}" | LC_ALL=C grep -qE '(^| / )Unknown( / |$)'; then
            die "go-licenses could not identify a license for some dependencies." \
                "Check the entries reported as Unknown before committing the file."
        fi
    done

    if cut -d, -f4 "${INDEX_FILE}" | LC_ALL=C grep -qx 'unknown'; then
        die "could not resolve module@version for some runtime packages from ${MODULES_TXT}." \
            "Run 'make vendor' and re-run, rather than committing a file with unattributed entries."
    fi

    if cut -d, -f2 "${TOOLS_INDEX}" | LC_ALL=C grep -qx 'Unknown'; then
        die "go-licenses could not resolve source URLs for some bundled-binary modules." \
            "This usually means the network blocked a '?go-get=1' lookup. Re-run with" \
            "access to the module hosts rather than committing a degraded file."
    fi
}

# Filter by name: for restricted licenses 'go-licenses save' copies the whole
# module source, which does not belong here.
license_files_for() {
    local dir="$1" f
    [[ -d "${dir}" ]] || return 0
    while IFS= read -r -d '' f; do
        # LC_ALL=C: under a Turkish locale glibc does not fold I to i, so this
        # stops matching LICENSE and every section renders as unavailable.
        if printf '%s' "$(basename "${f}")" \
            | LC_ALL=C grep -qiE '^(licen[cs]e|notice|copying|copyright|authors|patents)([-._].*)?$'; then
            printf '%s\n' "${f}"
        fi
    done < <(find "${dir}" -maxdepth 1 -type f -print0 2>/dev/null | LC_ALL=C sort -z)
}

emit_index_table() {
    local index="$1" provenance="$2" pkg url license module
    if [[ "${provenance}" == "module" ]]; then
        printf '| Package | License | Module |\n'
    else
        printf '| Package | License | Source |\n'
    fi
    printf '|---------|---------|--------|\n'

    while IFS=, read -r pkg url license module; do
        [[ -z "${pkg}" ]] && continue
        # shellcheck disable=SC2016  # backticks are literal markdown here.
        if [[ "${provenance}" == "module" ]]; then
            printf '| `%s` | %s | `%s` |\n' "${pkg}" "${license:-Unknown}" "${module:-unknown}"
        else
            printf '| `%s` | %s | %s |\n' "${pkg}" "${license:-Unknown}" "${url:-n/a}"
        fi
    done < "${index}"
}

emit_sections() {
    local index="$1" root="$2" provenance="$3"
    local pkg url license module files lf fence

    while IFS=, read -r pkg url license module; do
        [[ -z "${pkg}" ]] && continue

        printf '### %s\n\n' "${pkg}"
        printf '* License: %s\n' "${license:-Unknown}"
        if [[ "${provenance}" == "module" ]]; then
            printf '* Module: %s\n\n' "${module:-unknown}"
        else
            printf '* Source: %s\n\n' "${url:-n/a}"
        fi

        files=()
        while IFS= read -r lf; do
            [[ -n "${lf}" ]] && files+=("${lf}")
        done < <(license_files_for "${root}/${pkg}")

        # An entry with no saved text attributes nothing, so fail rather than
        # emit a placeholder that the check would then accept forever.
        (( ${#files[@]} > 0 )) || die "no license text was saved for ${pkg}." \
            "go-licenses classified it but saved no file. Check the package's" \
            "license layout before committing an entry without its text."
        for lf in "${files[@]}"; do
            fence="$(fence_for "${lf}")"
            printf '#### %s\n\n' "$(basename "${lf}")"
            printf '%stext\n' "${fence}"
            cat "${lf}"
            echo
            printf '%s\n' "${fence}"
            echo
        done
        echo
    done < "${index}"
}

compose_document() {
    log "Composing ${OUTPUT}..."
    {
        cat <<'EOF'
# Third-Party Notices

NVIDIA MIG Parted

This file lists every third-party dependency that MIG Parted redistributes,
along with the verbatim text of each dependency's license. In particular, this
covers all **Go modules** statically linked into the commands under `cmd/`. The
`nvidia-mig-parted` and `nvidia-mig-manager` commands ship in the
`k8s-mig-manager` image, and `nvidia-mig-parted` also ships in the deb, rpm and
tarball packages. The image additionally bundles `nvidia-ctk`, built from the
version pinned in `deployments/devel/go.mod`. Go standard library packages are
excluded; they are covered by the license of the Go distribution itself.

The `k8s-mig-manager` image uses `nvcr.io/nvidia/distroless/go` as a base image.
All of the OSS packages and source included in this image can be found at
<https://developer.nvidia.com/w/distroless-oss/index.html>. A statically
compiled busybox binary is added to the image, which is licensed under GPLv2.

## Runtime Dependency Index

EOF
        emit_index_table "${INDEX_FILE}" module

        cat <<'EOF'

## Bundled Binary Dependency Index

EOF
        emit_index_table "${TOOLS_INDEX}" source

        cat <<'EOF'

## Runtime Dependency License Texts

EOF
        emit_sections "${INDEX_FILE}" "${LICENSES_DIR}" module

        cat <<'EOF'
## Bundled Binary License Texts

EOF
        emit_sections "${TOOLS_INDEX}" "${LICENSES_DIR}/.tools" source
    } > "${OUT_TMP}"
    # mktemp creates 0600; set the mode before, never after, the rename.
    chmod 644 "${OUT_TMP}"
    # mv, not cp: rename(2) is atomic, so a failed run cannot truncate the
    # committed file.
    mv -f "${OUT_TMP}" "${OUTPUT}"
}

main() {
    check_prerequisites
    verify_platform_matrix
    prepare_workspace

    collect_runtime
    collect_bundled
    build_indexes
    compose_document

    local runtime_count bundled_count
    runtime_count=$(wc -l < "${INDEX_FILE}" | tr -d ' ')
    bundled_count=$(wc -l < "${TOOLS_INDEX}" | tr -d ' ')
    log "Wrote ${OUTPUT} (${runtime_count} runtime rows, ${bundled_count} bundled binary rows)"
}

main "$@"
