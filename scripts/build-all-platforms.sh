#!/bin/sh
set -eu

script_dir="$(dirname "$0")"
repo_root="$(CDPATH= cd -P "${script_dir}/.." && pwd -P)"
build_dir="${repo_root}/build"
target_platforms="darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64"
version="${VERSION:-dev}"
goexperiment="${GOEXPERIMENT:-}"

case "${version}" in
	*[!A-Za-z0-9._+-]*)
		echo "Invalid VERSION: ${version}" >&2
		exit 1
		;;
esac

echo "GOEXPERIMENT=${goexperiment}"

rm -rf "${build_dir}"
mkdir -p "${build_dir}"

for platform in ${target_platforms}; do
	os="${platform%/*}"
	arch="${platform#*/}"
	platform_dir="${build_dir}/${os}_${arch}"
	binary_name="ocsf-toolkit"

	if [ "${os}" = "windows" ]; then
		binary_name="${binary_name}.exe"
	fi

	echo "Building ocsf-toolkit for ${os}/${arch}"
	mkdir -p "${platform_dir}"
	GOOS="${os}" GOARCH="${arch}" CGO_ENABLED=0 GOEXPERIMENT="${goexperiment}" \
		go build -C "${repo_root}/cmd/ocsf-toolkit" -o "${platform_dir}/${binary_name}" -trimpath -ldflags "-X main.version=${version}"
done
