#!/bin/sh
set -eu

script_dir="$(dirname "$0")"
repo_root="$(CDPATH= cd -P "${script_dir}/.." && pwd -P)"
hostile_version='v1";id;echo"'

for target in build-all-platforms package; do
	output="$(make -n --no-print-directory -C "${repo_root}" "${target}" "VERSION=${hostile_version}")"
	case "${output}" in
	*';id;'*)
		echo "Make target ${target} expands VERSION as shell syntax" >&2
		exit 1
		;;
	esac
done
