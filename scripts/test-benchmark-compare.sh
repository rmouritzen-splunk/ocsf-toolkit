#!/bin/sh
set -eu

script_dir="$(dirname "$0")"
repo_root="$(CDPATH= cd -P "${script_dir}/.." && pwd -P)"

help_output="$(BENCHMARK_BASE=not-a-release "${repo_root}/scripts/benchmark-compare.sh" --help)"
case "${help_output}" in
*"Usage: scripts/benchmark-compare.sh"*"--base"*"--count"*"--time"*"--pattern"*) ;;
*)
	echo "Benchmark comparison help does not describe its command arguments" >&2
	exit 1
	;;
esac

if "${repo_root}/scripts/benchmark-compare.sh" --unknown >/dev/null 2>&1; then
	echo "Benchmark comparison accepted an unknown option" >&2
	exit 1
fi

if "${repo_root}/scripts/benchmark-compare.sh" --count >/dev/null 2>&1; then
	echo "Benchmark comparison accepted an option without its required argument" >&2
	exit 1
fi
