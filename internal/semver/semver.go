// Package semver parses and compares semantic versions without a leading v.
package semver

import "strings"

// Pattern describes the complete Semantic Versioning 2.0.0 syntax accepted by Parse.
const Pattern = `^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` + // major.minor.patch
	`(?:-((?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*)(?:\.(?:0|[1-9]\d*|\d*[A-Za-z-][0-9A-Za-z-]*))*))?` + // prerelease
	`(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$` // build metadata

// Version is a parsed semantic version. Its substrings refer to the immutable input string.
type Version struct {
	major      string
	minor      string
	patch      string
	prerelease string
}

// Parse parses a complete Semantic Versioning 2.0.0 version without a leading v.
func Parse(value string) (Version, bool) {
	position := 0
	major, next, ok := numericIdentifier(value, position)
	if !ok || next >= len(value) || value[next] != '.' {
		return Version{}, false
	}
	position = next + 1
	minor, next, ok := numericIdentifier(value, position)
	if !ok || next >= len(value) || value[next] != '.' {
		return Version{}, false
	}
	position = next + 1
	patch, position, ok := numericIdentifier(value, position)
	if !ok {
		return Version{}, false
	}

	prerelease := ""
	if position < len(value) && value[position] == '-' {
		start := position + 1
		position, ok = identifierList(value, start, true)
		if !ok {
			return Version{}, false
		}
		prerelease = value[start:position]
	}
	if position < len(value) && value[position] == '+' {
		position, ok = identifierList(value, position+1, false)
		if !ok {
			return Version{}, false
		}
	}
	if position != len(value) {
		return Version{}, false
	}
	return Version{major: major, minor: minor, patch: patch, prerelease: prerelease}, true
}

func numericIdentifier(value string, start int) (string, int, bool) {
	position := start
	for position < len(value) && isDigit(value[position]) {
		position++
	}
	if position == start || position-start > 1 && value[start] == '0' {
		return "", start, false
	}
	return value[start:position], position, true
}

func identifierList(value string, start int, numericLeadingZeroInvalid bool) (int, bool) {
	position := start
	identifierStart := start
	numeric := true
	for {
		if position == len(value) || value[position] == '+' && numericLeadingZeroInvalid {
			if position == identifierStart || numericLeadingZeroInvalid && numeric &&
				position-identifierStart > 1 && value[identifierStart] == '0' {
				return start, false
			}
			return position, true
		}
		character := value[position]
		if character == '.' {
			if position == identifierStart || numericLeadingZeroInvalid && numeric &&
				position-identifierStart > 1 && value[identifierStart] == '0' {
				return start, false
			}
			position++
			identifierStart = position
			numeric = true
			continue
		}
		if !isIdentifierCharacter(character) {
			return start, false
		}
		numeric = numeric && isDigit(character)
		position++
	}
}

func isDigit(character byte) bool {
	return character >= '0' && character <= '9'
}

func isIdentifierCharacter(character byte) bool {
	return isDigit(character) || character >= 'A' && character <= 'Z' ||
		character >= 'a' && character <= 'z' || character == '-'
}

// Compare compares versions according to Semantic Versioning precedence. Build metadata does not affect precedence.
func Compare(left, right Version) int {
	if comparison := compareNumericIdentifier(left.major, right.major); comparison != 0 {
		return comparison
	}
	if comparison := compareNumericIdentifier(left.minor, right.minor); comparison != 0 {
		return comparison
	}
	if comparison := compareNumericIdentifier(left.patch, right.patch); comparison != 0 {
		return comparison
	}
	return comparePrerelease(left.prerelease, right.prerelease)
}

func compareNumericIdentifier(left, right string) int {
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return strings.Compare(left, right)
}

func comparePrerelease(left, right string) int {
	if left == "" {
		if right == "" {
			return 0
		}
		return 1
	}
	if right == "" {
		return -1
	}
	for {
		leftIdentifier, leftRemainder, leftMore := strings.Cut(left, ".")
		rightIdentifier, rightRemainder, rightMore := strings.Cut(right, ".")
		leftNumeric := identifierNumeric(leftIdentifier)
		rightNumeric := identifierNumeric(rightIdentifier)
		var comparison int
		switch {
		case leftNumeric && rightNumeric:
			comparison = compareNumericIdentifier(leftIdentifier, rightIdentifier)
		case leftNumeric:
			comparison = -1
		case rightNumeric:
			comparison = 1
		default:
			comparison = strings.Compare(leftIdentifier, rightIdentifier)
		}
		if comparison != 0 {
			return comparison
		}
		if !leftMore || !rightMore {
			switch {
			case leftMore:
				return 1
			case rightMore:
				return -1
			default:
				return 0
			}
		}
		left = leftRemainder
		right = rightRemainder
	}
}

func identifierNumeric(identifier string) bool {
	for index := range len(identifier) {
		if !isDigit(identifier[index]) {
			return false
		}
	}
	return true
}

// IsInitialDevelopment reports whether the major version is zero.
func (v Version) IsInitialDevelopment() bool {
	return v.major == "0"
}

// IsPrerelease reports whether the version has prerelease identifiers.
func (v Version) IsPrerelease() bool {
	return v.prerelease != ""
}
