# Release Process

OCSF Toolkit releases are Git tag driven. A release is created by pushing a version tag such as `v0.1.0`; GitHub Actions builds the release artifacts and publishes the GitHub Release.

## Access

The person creating a release must be able to push tags to the repository at <https://github.com/ocsf/ocsf-toolkit>. In GitHub's standard repository roles, this means Write, Maintain, or Admin access. Protect `v*` tags with a repository ruleset so only the release-maintainer group can create release tags.

Recommended repository policy:

- Protect the `main` branch and require pull requests.
- Restrict creation of `v*` tags to release maintainers.
- Disallow deleting or force-updating release tags where GitHub rulesets allow it.

## Version Tags

Use annotated tags. Release tags should start with `v` and use semantic versioning. Do not include SemVer build metadata (`+...`) in release tags.

Examples:

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

Prerelease tags should include a prerelease suffix:

```sh
git tag -a v0.1.0-rc.1 -m "v0.1.0-rc.1"
git push origin v0.1.0-rc.1
```

The release workflow marks tags containing `-` as GitHub prereleases.

## Before Tagging

Release from a clean worktree on the commit that should become the release.

```sh
git status --short
make package VERSION=v0.1.0
```

The local `make package` run is a preflight check. It runs the complete `all` target with release build settings, rebuilds every supported platform, and packages the resulting platform directories. The cross-platform build script owns the supported-platform list; packaging archives the contents it produces rather than maintaining another list. Prefer running the preflight with the same Go version used by the release workflow when practical. The official release artifacts are produced by GitHub Actions after the tag is pushed.

Inspect local `dist/` if needed. It should contain archives for each target platform and `SHA256SUMS`.

## Publishing

After local verification, tag the same commit and push the tag:

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

GitHub Actions runs `.github/workflows/release.yml`. The workflow:

- Rejects an obvious mistyped release tag unless it begins with `v` followed by a digit. This is a typo guard, not full semantic-version validation.
- Sets up the release Go toolchain, currently Go 1.27.x (latest Go 1.27).
- Installs the pinned verification tools.
- Runs `make package VERSION="${GITHUB_REF_NAME}"`, including all checks, tests, and cross-platform builds.
- Creates the GitHub Release.
- Uploads `dist/*`.

The development-tools module also declares Go 1.27 so local verification, CI verification, benchmark tooling, and release tooling share one current toolchain baseline. The separately supported Go 1.25 baseline applies to the main library and CLI module; CI checks it with a focused `go test ./...` run that disables automatic toolchain switching.

Do not create the GitHub Release manually before pushing the tag. The workflow creates the release; if a release already exists, publishing may fail.

## After Publishing

Check the GitHub Actions run and the created GitHub Release. Confirm that the release includes:

- One archive per target platform.
- `SHA256SUMS`.
- Generated release notes.

Optionally download one artifact and check:

```sh
ocsf-toolkit --version
```

The version should match the tag.

## Correcting A Release

For a release that has not been announced or consumed, the cleanest correction is usually:

1. Delete the GitHub Release.
2. Delete the remote tag.
3. Delete the local tag.
4. Fix the issue.
5. Create and push the tag again.

Commands:

```sh
gh release delete v0.1.0 --yes --cleanup-tag
git tag -d v0.1.0
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

For a release that has already been consumed, do not replace the tag. Publish a new patch version instead.
