package validation

import "github.com/ocsf/ocsf-toolkit/internal/coderegistry"

// Level identifies the effective reporting level of a validation finding.
type Level uint8

const (
	invalidLevel Level = iota
	// LevelWarning reports a condition that should be reviewed but does not fail validation under the active policy.
	LevelWarning
	// LevelError reports a condition that fails validation under the active policy.
	LevelError
	levelCount
)

var levelInfos = [levelCount]coderegistry.Info{
	LevelWarning: {Name: "warning"},
	LevelError:   {Name: "error"},
}

var levelRegistry = coderegistry.New[Level]("validation level", levelInfos[:])

// Valid reports whether level is defined by this toolkit version.
func (level Level) Valid() bool {
	return levelRegistry.Valid(level)
}

// String returns the stable external representation of level, or an empty string for an invalid level.
func (level Level) String() string {
	return levelRegistry.String(level)
}

// ParseLevel resolves a stable external validation-level representation.
func ParseLevel(value string) (Level, bool) {
	return levelRegistry.Parse(value)
}

// MarshalText returns the stable external representation used by JSON encoders.
func (level Level) MarshalText() ([]byte, error) {
	return levelRegistry.MarshalText(level)
}

// UnmarshalText resolves a stable external representation used by JSON decoders.
func (level *Level) UnmarshalText(text []byte) error {
	return levelRegistry.UnmarshalText(text, level)
}
