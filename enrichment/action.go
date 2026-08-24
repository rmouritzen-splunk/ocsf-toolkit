package enrichment

// Action selects what happens to one kind of enrichment (enum siblings or observables) in an enrichment processor.
type Action string

const (
	// None leaves the enrichment as-is.
	None Action = "none"
	// Add adds the enrichment.
	Add Action = "add"
	// Remove removes the enrichment, verified against the schema.
	Remove Action = "remove"
	// ForceRemove removes the enrichment without verifying it against the schema.
	ForceRemove Action = "force-remove"
)

// Valid reports whether the action is supported.
func (a Action) Valid() bool {
	switch a {
	case None, Add, Remove, ForceRemove:
		return true
	default:
		return false
	}
}

// IsRemoval reports whether the action removes enrichment (Remove or ForceRemove).
func (a Action) IsRemoval() bool {
	return a == Remove || a == ForceRemove
}
