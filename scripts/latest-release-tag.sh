#!/bin/sh
set -eu

requested_tag="${1:-}"

eligible_tag() {
	# This intentionally mirrors the release workflow's lightweight typo guard rather than duplicating the complete
	# SemVer parser. Release tags are maintainer-controlled; benchmark selection needs the same acceptance policy.
	case "$1" in
		v[0-9]*) ;;
		*) return 1 ;;
	esac
	case "$1" in
		*+*) return 1 ;;
	esac
	git merge-base --is-ancestor "$1^{commit}" HEAD 2>/dev/null
}

if [ -n "${requested_tag}" ]; then
	if ! eligible_tag "${requested_tag}"; then
		echo "Benchmark base is not an eligible release tag reachable from HEAD: ${requested_tag}" >&2
		exit 1
	fi
	printf '%s\n' "${requested_tag}"
	exit 0
fi

# Prereleases intentionally participate in this single newest-release baseline. If published
# prereleases become part of the final release strategy, resolve both the newest prerelease and
# newest stable release here and compare against both.
# Treat a dash suffix as a prerelease so a final tag for the same version sorts newer.
for tag in $(git -c versionsort.suffix=- -c versionsort.suffix= \
	tag --merged HEAD --list 'v[0-9]*' --sort=-version:refname); do
	if eligible_tag "${tag}"; then
		printf '%s\n' "${tag}"
		exit 0
	fi
done

echo "No eligible release tag is reachable from HEAD." >&2
exit 1
