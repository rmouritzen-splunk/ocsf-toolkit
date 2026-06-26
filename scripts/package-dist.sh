#!/bin/sh
set -eu

script_dir="$(dirname "$0")"
repo_root="$(CDPATH= cd "${script_dir}/.." && pwd)"
build_dir="${BUILD_DIR:-"$repo_root/build"}"
dist_dir="${DIST_DIR:-"$repo_root/dist"}"
target_platforms="${TARGET_PLATFORMS:-darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64}"
version="${VERSION:-dev}"
project_name="ocsf-toolkit"

case "${version}" in
	*[!A-Za-z0-9._+-]*)
		echo "Invalid VERSION: ${version}" >&2
		exit 1
		;;
esac

case "${dist_dir}" in
	"" | "/")
		echo "Refusing to use unsafe DIST_DIR: ${dist_dir}" >&2
		exit 1
		;;
esac

rm -rf "${dist_dir}"
mkdir -p "${dist_dir}"

staging_dir="${dist_dir}/.staging"
mkdir -p "${staging_dir}"
trap 'rm -rf "${staging_dir}"' EXIT HUP INT TERM

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
	platform_name="${os}_${arch}"
	binary_name="${project_name}"
	archive_base="${project_name}_${version}_${platform_name}"

	if [ "${os}" = "windows" ]; then
		binary_name="${binary_name}.exe"
	fi

	binary_path="${build_dir}/${platform_name}/${binary_name}"
	if [ ! -f "${binary_path}" ]; then
		echo "Missing release binary: ${binary_path}" >&2
		exit 1
	fi

	package_root="${staging_dir}/${archive_base}"
	rm -rf "${package_root}"
	mkdir -p "${package_root}"

	cp "${binary_path}" "${package_root}/"
	chmod 0755 "${package_root}/${binary_name}"

	for file in README.md LICENSE NOTICE THIRD_PARTY_LICENSES.md; do
		if [ -f "${repo_root}/${file}" ]; then
			cp "${repo_root}/${file}" "${package_root}/"
		fi
	done

	if [ "${os}" = "windows" ]; then
		echo "Creating ${archive_base}.zip"
		(cd "${staging_dir}" && zip -qr "${dist_dir}/${archive_base}.zip" "${archive_base}")
	else
		echo "Creating ${archive_base}.tar.gz"
		tar -czf "${dist_dir}/${archive_base}.tar.gz" -C "${staging_dir}" "${archive_base}"
	fi
done

(cd "${dist_dir}" && shasum -a 256 "${project_name}"_* > SHA256SUMS)
