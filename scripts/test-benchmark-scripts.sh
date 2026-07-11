#!/bin/sh
set -eu

script_dir="$(dirname "$0")"
repo_root="$(CDPATH= cd -P "${script_dir}/.." && pwd -P)"
test_root="$(mktemp -d "${TMPDIR:-/tmp}/ocsf-toolkit-benchmark-scripts.XXXXXX")"
trap 'rm -rf "${test_root}"' EXIT HUP INT TERM

repo="${test_root}/repo"
git init -q "${repo}"
default_branch="$(git -C "${repo}" branch --show-current)"
git -C "${repo}" config user.email test@example.com
git -C "${repo}" config user.name Test
mkdir -p "${repo}/scripts"
cp "${repo_root}/scripts/latest-release-tag.sh" "${repo}/scripts/"

commit_file="${repo}/content"
printf 'one\n' >"${commit_file}"
git -C "${repo}" add content
git -C "${repo}" commit -qm one
if (cd "${repo}" && scripts/latest-release-tag.sh >/dev/null 2>&1); then
	echo "Expected repository without release tags to have no benchmark base" >&2
	exit 1
fi
git -C "${repo}" tag -a v0.1.0 -m v0.1.0

printf 'two\n' >>"${commit_file}"
git -C "${repo}" commit -qam two
git -C "${repo}" tag -a v0.2.0-rc.1 -m v0.2.0-rc.1
git -C "${repo}" tag -a v9+invalid -m invalid
git -C "${repo}" tag -a invalid-v10.0.0 -m invalid

actual="$(cd "${repo}" && scripts/latest-release-tag.sh)"
test "${actual}" = "v0.2.0-rc.1"

git -C "${repo}" tag -a v0.2.0 -m v0.2.0
actual="$(cd "${repo}" && scripts/latest-release-tag.sh)"
test "${actual}" = "v0.2.0"

actual="$(cd "${repo}" && scripts/latest-release-tag.sh v0.1.0)"
test "${actual}" = "v0.1.0"

if (cd "${repo}" && scripts/latest-release-tag.sh v9+invalid >/dev/null 2>&1); then
	echo "Expected explicit tag containing + to be rejected" >&2
	exit 1
fi

git -C "${repo}" switch -qc unrelated v0.1.0
printf 'unrelated\n' >>"${commit_file}"
git -C "${repo}" commit -qam unrelated
git -C "${repo}" tag -a v1.0.0 -m v1.0.0
git -C "${repo}" switch -q "${default_branch}"

if (cd "${repo}" && scripts/latest-release-tag.sh v1.0.0 >/dev/null 2>&1); then
	echo "Expected unreachable explicit tag to be rejected" >&2
	exit 1
fi
