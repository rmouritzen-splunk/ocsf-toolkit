# Homebrew Packaging

Homebrew support should start with source-built formulae in a dedicated OCSF tap repository, not casks or formulae embedded in individual project repositories. `ocsf-toolkit` can be the first formula, with `ocsf-schema-compiler` added later.

## Recommended Tap

Create the shared repository `github.com/ocsf/homebrew-tap`. Users would install the toolkit with:

```sh
brew tap ocsf/tap
brew install ocsf-toolkit
```

The same tap can later provide `ocsf-schema-compiler`. Because that tool is a Python application available on PyPI, its formula will likely use a different packaging pattern from the Go source-built `ocsf-toolkit` formula.

## Formula, Not Cask

Use a formula for the initial implementation. Building the Go CLI from source is the normal Homebrew pattern and avoids the unsigned-binary quarantine warning that applies to downloaded GitHub release binaries.

A cask would install a precompiled upstream binary and would retain the current signing and notarization limitations.

## Formula Shape

The tap repository would contain `Formula/ocsf-toolkit.rb`:

```ruby
class OcsfToolkit < Formula
  desc "Process OCSF event files with a compiled OCSF schema"
  homepage "https://github.com/ocsf/ocsf-toolkit"
  url "https://github.com/ocsf/ocsf-toolkit/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "..."
  license "Apache-2.0"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{version}"
    system "go", "build", *std_go_args(
      output: bin/"ocsf-toolkit",
      ldflags: ldflags,
    ), "./cmd/ocsf-toolkit"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/ocsf-toolkit --version")
  end
end
```

Verify the exact linker variable before implementing the formula; it must match the CLI's current version variable.

## Versions

Homebrew does not automatically select the latest GitHub tag for a formula. Each release updates the source URL and SHA-256 checksum:

```ruby
url "https://github.com/ocsf/ocsf-toolkit/archive/refs/tags/v0.1.0.tar.gz"
sha256 "..."
```

Homebrew normally infers `0.1.0` from the tag URL. Set `version` explicitly only if inference is wrong or unclear.

Suggested release flow:

1. Publish a version tag from `ocsf/ocsf-toolkit` and allow this repository's release workflow to create GitHub artifacts.
2. Compute the SHA-256 checksum of the GitHub source archive for that tag.
3. Update `Formula/ocsf-toolkit.rb` in `ocsf/homebrew-tap`.
4. Run `brew install --build-from-source ./Formula/ocsf-toolkit.rb` and `brew test ./Formula/ocsf-toolkit.rb`.
5. Push the tap update.

The tap update can be automated later.

## Bottles And Signing

Start by compiling locally. Go CLIs generally build quickly, and Homebrew installs Go as a build dependency when needed.

Bottles can later provide faster installation. A tap workflow would build and upload them using Homebrew's test-bot and bottle tooling. Bottle checksums provide integrity verification, not Apple notarization or Windows Authenticode signing.

Homebrew support does not itself provide OS-level code signing:

- A source-built formula creates the binary locally on the user's machine.
- A bottle installs a checked prebuilt Homebrew package but is not equivalent to OS signing.
- A cask installs the upstream binary and therefore retains its signing status.

If OCSF later provides OS-trusted binaries, the project will still need Apple Developer signing and notarization plus Windows signing infrastructure. GitHub Actions can run those workflows but does not provide the signing identities.

Start with `ocsf/homebrew-tap`. Consider submission to `Homebrew/homebrew-core` only after the project has stable releases, public adoption, and formula behavior that meets Homebrew/core requirements.
