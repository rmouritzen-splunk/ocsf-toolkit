package semver

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseAcceptsCompleteSemVerSyntax(t *testing.T) {
	valid := []string{
		"0.0.0",
		"1.8.0",
		"1.8.0-rc.1",
		"1.8.0-release-with-dashes",
		"1.8.0-prerelease+metadata",
		"1.8.0+build.1",
		"1.8.0-alpha.beta.1",
		"999999999999999999999999.0.1",
		"1.0.0-0.3.7",
		"1.0.0-x.7.z.92",
		"1.0.0-x-y-z.--",
		"1.0.0+21AF26D3----117B344092BD",
	}
	invalid := []string{
		"",
		"v1.8.0",
		"1",
		"1.8",
		"1.8.0.1",
		"01.8.0",
		"1.08.0",
		"1.8.00",
		"1.8.0-",
		"1.8.0-01",
		"1.8.0+",
		"1.8.0_prerelease",
		"1.0.0-alpha..1",
		"1.0.0-alpha.",
		"1.0.0+build..1",
		"1.0.0+build.",
		"1.0.0-alpha+build+second",
		"1.0.0-α",
	}

	pattern := regexp.MustCompile(Pattern)
	for _, value := range valid {
		_, ok := Parse(value)
		require.True(t, ok, value)
		require.True(t, pattern.MatchString(value), value)
	}
	for _, value := range invalid {
		_, ok := Parse(value)
		require.False(t, ok, value)
		require.False(t, pattern.MatchString(value), value)
	}
}

func TestCompareUsesSemVerPrecedence(t *testing.T) {
	parse := func(value string) Version {
		version, ok := Parse(value)
		require.True(t, ok)
		return version
	}

	require.Less(t, Compare(parse("1.0.0-rc.2"), parse("1.0.0-rc.10")), 0)
	require.Less(t, Compare(parse("1.0.0-rc.10"), parse("1.0.0")), 0)
	require.Zero(t, Compare(parse("1.0.0+first"), parse("1.0.0+second")))
	require.True(t, parse("0.9.0").IsInitialDevelopment())
	require.True(t, parse("1.0.0-rc.1").IsPrerelease())
	require.Less(t, Compare(parse("1.0.0-alpha"), parse("1.0.0-alpha.1")), 0)
	require.Less(t, Compare(parse("1.0.0-alpha.1"), parse("1.0.0-alpha.beta")), 0)
	require.Less(t, Compare(parse("1.0.0-beta.2"), parse("1.0.0-beta.11")), 0)
	require.Less(t, Compare(parse("999999999999999999999999.0.0"), parse("1000000000000000000000000.0.0")), 0)
}
