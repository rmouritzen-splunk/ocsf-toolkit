package issue

import "github.com/ocsf/ocsf-toolkit/internal/coderegistry"

// Level controls how an event-processing issue affects pipeline processing.
type Level uint8

const (
	// LevelIgnored omits an ignorable issue and continues processing.
	LevelIgnored Level = iota + 1
	// LevelWarning reports an issue in the processing result and continues processing.
	LevelWarning
	// LevelError returns an issue as a processing error and stops processing.
	LevelError
	levelCount
)

var levelInfos = [levelCount]coderegistry.Info{
	LevelIgnored: {Name: "ignored"},
	LevelWarning: {Name: "warning"},
	LevelError:   {Name: "error"},
}

var levelRegistry = coderegistry.New[Level]("issue level", levelInfos[:])

// Valid reports whether level is defined by this toolkit version.
func (level Level) Valid() bool {
	return levelRegistry.Valid(level)
}

// String returns the stable external representation of level, or an empty string for an invalid level.
func (level Level) String() string {
	return levelRegistry.String(level)
}

// ParseLevel resolves a stable external issue-level representation.
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
