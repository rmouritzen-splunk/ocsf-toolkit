#!/bin/sh
set -eu

script_dir="$(dirname "$0")"
repo_root="$(CDPATH= cd -P "${script_dir}/.." && pwd -P)"
benchmark_base=""
benchmark_count=10
benchmark_time=500ms
benchmark_pattern=.

usage() {
	printf '%s\n' \
		"Usage: scripts/benchmark-compare.sh [options]" \
		"" \
		"Options:" \
		"  --base TAG       Compare with a specific reachable release tag." \
		"  --count N        Run each benchmark N times (default: 10)." \
		"  --time DURATION  Run each benchmark for DURATION (default: 500ms)." \
		"  --pattern REGEXP Run benchmarks matching REGEXP (default: .)." \
		"  -h, --help       Show this help."
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--base | --count | --time | --pattern)
		option="$1"
		if [ "$#" -lt 2 ]; then
			echo "Option ${option} requires an argument." >&2
			exit 2
		fi
		case "${option}" in
		--base) benchmark_base="$2" ;;
		--count) benchmark_count="$2" ;;
		--time) benchmark_time="$2" ;;
		--pattern) benchmark_pattern="$2" ;;
		esac
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "Unknown option: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

if [ -n "${benchmark_base}" ]; then
	benchmark_base="$(${repo_root}/scripts/latest-release-tag.sh "${benchmark_base}")"
else
	if ! benchmark_base="$(${repo_root}/scripts/latest-release-tag.sh)"; then
		echo "No eligible release tag is reachable from HEAD; benchmark comparison skipped." >&2
		exit 0
	fi
fi

base_commit="$(git -C "${repo_root}" rev-parse "${benchmark_base}^{commit}")"
head_commit="$(git -C "${repo_root}" rev-parse HEAD)"
if [ "${base_commit}" = "${head_commit}" ] &&
	[ -z "$(git -C "${repo_root}" status --porcelain --untracked-files=normal)" ]; then
	echo "HEAD matches ${benchmark_base}; benchmark comparison skipped."
	exit 0
fi

work_root="$(mktemp -d "${TMPDIR:-/tmp}/ocsf-toolkit-benchmark.XXXXXX")"
base_worktree="${work_root}/base"
base_output="${work_root}/base.txt"
current_output="${work_root}/current.txt"
cleanup() {
	git -C "${repo_root}" worktree remove --force "${base_worktree}" >/dev/null 2>&1 || true
	rm -rf "${work_root}"
}
trap cleanup EXIT HUP INT TERM

git -C "${repo_root}" worktree add --detach "${base_worktree}" "${base_commit}" >/dev/null

echo "Benchmarking release baseline ${benchmark_base} (${base_commit})"
(cd "${base_worktree}" && go test ./eventschema -run '^$' -bench "${benchmark_pattern}" \
	-benchmem -benchtime "${benchmark_time}" -count "${benchmark_count}") >"${base_output}"

echo "Benchmarking current checkout (${head_commit})"
(cd "${repo_root}" && go test ./eventschema -run '^$' -bench "${benchmark_pattern}" \
	-benchmem -benchtime "${benchmark_time}" -count "${benchmark_count}") >"${current_output}"

echo "Comparing ${benchmark_base} with current checkout"
cd "${repo_root}/tools"
go tool benchstat "release=${base_output}" "current=${current_output}"
