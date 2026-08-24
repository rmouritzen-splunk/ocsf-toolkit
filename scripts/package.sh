#!/bin/sh
set -eu

script_dir="$(dirname "$0")"
repo_root="$(CDPATH= cd -P "${script_dir}/.." && pwd -P)"
build_dir="${repo_root}/build"
dist_dir="${repo_root}/dist"
version="${VERSION:-dev}"
project_name="ocsf-toolkit"

case "${version}" in
	*[!A-Za-z0-9._+-]*)
		echo "Invalid VERSION: ${version}" >&2
		exit 1
		;;
esac

rm -rf "${dist_dir}"
mkdir -p "${dist_dir}"

staging_dir="${dist_dir}/.staging"
mkdir -p "${staging_dir}"
trap 'rm -rf "${staging_dir}"' EXIT HUP INT TERM

found_platform=false
for platform_dir in "${build_dir}"/*; do
	if [ ! -d "${platform_dir}" ]; then
		continue
	fi

	found_platform=true
	platform_name="$(basename "${platform_dir}")"
	archive_base="${project_name}_${version}_${platform_name}"

	package_root="${staging_dir}/${archive_base}"
	rm -rf "${package_root}"
	mkdir -p "${package_root}"

	cp -R "${platform_dir}/." "${package_root}/"

	for file in README.md LICENSE NOTICE THIRD_PARTY_LICENSES.md; do
		if [ -f "${repo_root}/${file}" ]; then
			cp "${repo_root}/${file}" "${package_root}/"
		fi
	done

	case "${platform_name}" in
	windows_*)
		echo "Creating ${archive_base}.zip"
		(cd "${staging_dir}" && zip -qr "${dist_dir}/${archive_base}.zip" "${archive_base}")
		;;
	*)
		echo "Creating ${archive_base}.tar.gz"
		tar -czf "${dist_dir}/${archive_base}.tar.gz" -C "${staging_dir}" "${archive_base}"
		;;
	esac
done

if [ "${found_platform}" = false ]; then
	echo "No platform builds found in ${build_dir}" >&2
	exit 1
fi

(cd "${dist_dir}" && shasum -a 256 "${project_name}"_* > SHA256SUMS)
