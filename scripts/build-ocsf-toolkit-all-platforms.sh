#!/bin/sh
set -eu

script_dir="$(dirname "$0")"
repo_root="$(CDPATH= cd "${script_dir}/.." && pwd)"
build_dir="${BUILD_DIR:-"$repo_root/build"}"
target_platforms="${TARGET_PLATFORMS:-darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64}"
version="${VERSION:-dev}"

case "${version}" in
	*[!A-Za-z0-9._+-]*)
		echo "Invalid VERSION: ${version}" >&2
		exit 1
		;;
esac

for platform in ${target_platforms}; do
	case "${platform}" in
		*/*) ;;
		*)
			echo "Invalid target platform: ${platform}" >&2
			exit 1
			;;
	esac

	os="${platform%/*}"
	arch="${platform#*/}"
	platform_dir="${build_dir}/${os}_${arch}"
	binary_name="ocsf-toolkit"

	if [ "${os}" = "windows" ]; then
		binary_name="${binary_name}.exe"
	fi

	echo "Building ocsf-toolkit for ${os}/${arch}"
	mkdir -p "${platform_dir}"
	GOOS="${os}" GOARCH="${arch}" CGO_ENABLED=0 go build -C "${repo_root}/cmd/ocsf-toolkit" -o "${platform_dir}/${binary_name}" -trimpath -ldflags "-X main.version=${version}"
done
