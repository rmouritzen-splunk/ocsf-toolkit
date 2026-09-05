package processing

// levelPolicy is the shared internal hot-loop representation of validation and processing-issue levels. Public codes
// remain ordinal registry values, while policy files define their corresponding bit masks as compile-time constants.
// This avoids repeated map lookups while ProcessEvent decides whether to skip ignored diagnostic work. Policy
// compilation places every valid code below 64 in exactly one level mask. If either public registry reaches code 64,
// this representation must be replaced with a wider one before that code is added.
type levelPolicy struct {
	ignored uint64
	warning uint64
	error   uint64
}

func (p levelPolicy) isIgnored(mask uint64) bool {
	// Shared work is ignorable only when every finding or issue it can emit is ignored.
	return p.ignored&mask == mask
}

func (p levelPolicy) isWarning(mask uint64) bool {
	return p.warning&mask != 0
}

func (p levelPolicy) isError(mask uint64) bool {
	return p.error&mask != 0
}
