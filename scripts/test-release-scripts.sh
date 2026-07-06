#!/bin/sh
set -eu

script_dir="$(dirname "$0")"
repo_root="$(CDPATH= cd -P "${script_dir}/.." && pwd -P)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/ocsf-toolkit-release-scripts.XXXXXX")"
trap 'rm -rf "${test_root}"' EXIT HUP INT TERM

test_repo="${test_root}/repo"
mkdir -p "${test_repo}/scripts" "${test_repo}/build" "${test_repo}/dist" "${test_repo}/docs"
cp "${repo_root}/scripts/build-ocsf-toolkit-all-platforms.sh" "${test_repo}/scripts/"
cp "${repo_root}/scripts/package-dist.sh" "${test_repo}/scripts/"

build_script="${test_repo}/scripts/build-ocsf-toolkit-all-platforms.sh"
package_script="${test_repo}/scripts/package-dist.sh"

expect_failure() {
	if "$@" >/dev/null 2>&1; then
		echo "Expected command to fail: $*" >&2
		exit 1
	fi
}

touch "${test_repo}/docs/marker"
expect_failure env BUILD_DIR="${test_repo}/docs" "${build_script}"
test -f "${test_repo}/docs/marker"

expect_failure env DIST_DIR="${test_repo}/docs" "${package_script}"
test -f "${test_repo}/docs/marker"

touch "${test_repo}/build/marker"
expect_failure env TARGET_PLATFORMS="linux/amd64/../../escape" "${build_script}"
test -f "${test_repo}/build/marker"

touch "${test_repo}/dist/marker"
expect_failure env TARGET_PLATFORMS="linux/amd64/../../escape" "${package_script}"
test -f "${test_repo}/dist/marker"
