package processing

import "github.com/ocsf/ocsf-toolkit/issue"

// issueSuppression is immutable pipeline-owned state. A zero value collects every issue, while configured with a nil
// code set suppresses every suppressible issue.
type issueSuppression struct {
	configured bool
	codes      map[issue.IssueCode]struct{}
}

func newIssueSuppression(config IssueSuppressionConfig) issueSuppression {
	if !config.Configured {
		return issueSuppression{}
	}
	if len(config.Codes) == 0 {
		return issueSuppression{configured: true}
	}
	codes := make(map[issue.IssueCode]struct{}, len(config.Codes))
	for _, code := range config.Codes {
		codes[code] = struct{}{}
	}
	return issueSuppression{configured: true, codes: codes}
}

func (s issueSuppression) suppresses(code issue.IssueCode) bool {
	if !s.configured {
		return false
	}
	if !code.Suppressible() {
		return false
	}
	if s.codes == nil {
		return true
	}
	_, suppressed := s.codes[code]
	return suppressed
}
