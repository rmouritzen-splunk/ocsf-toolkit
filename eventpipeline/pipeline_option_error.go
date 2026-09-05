package eventpipeline

import (
	"fmt"

	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/validation"
)

// PipelineOptionName identifies an option involved in invalid pipeline configuration.
type PipelineOptionName string

const (
	PipelineOptionSchema                           PipelineOptionName = "schema"
	PipelineOptionEnumSiblings                     PipelineOptionName = "enum_siblings"
	PipelineOptionObservables                      PipelineOptionName = "observables"
	PipelineOptionEnrichmentObservablePathNotation PipelineOptionName = "enrichment_observable_path_notation"
	PipelineOptionValidation                       PipelineOptionName = "validation"
	PipelineOptionValidationObservablePathNotation PipelineOptionName = "validation_observable_path_notation"
	PipelineOptionIssueLevel                       PipelineOptionName = "issue_level"
	PipelineOptionAllIssueLevels                   PipelineOptionName = "all_issue_levels"
	PipelineOptionValidationLevel                  PipelineOptionName = "validation_level"
	PipelineOptionAllValidationLevels              PipelineOptionName = "all_validation_levels"
)

// PipelineOptionError reports invalid pipeline-option configuration. Its implementations identify the specific
// configuration problem and expose any associated option or code through typed accessors.
type PipelineOptionError interface {
	error
	Option() PipelineOptionName
	isPipelineOptionError()
}

type pipelineOptionError struct {
	option PipelineOptionName
}

func (e pipelineOptionError) Option() PipelineOptionName {
	return e.option
}

func (pipelineOptionError) isPipelineOptionError() {}

// PipelineOptionDuplicateError reports a pipeline option that was configured more than once.
type PipelineOptionDuplicateError struct {
	pipelineOptionError
}

func newPipelineOptionDuplicateError(option PipelineOptionName) *PipelineOptionDuplicateError {
	return &PipelineOptionDuplicateError{pipelineOptionError: pipelineOptionError{option: option}}
}

func (e *PipelineOptionDuplicateError) Error() string {
	return fmt.Sprintf("pipeline option %s may be specified only once", e.Option())
}

// PipelineOptionIssueLevelAllAfterCodeError reports an all-code issue level configured after a specific issue code.
type PipelineOptionIssueLevelAllAfterCodeError struct {
	pipelineOptionError
}

func newPipelineOptionIssueLevelAllAfterCodeError() *PipelineOptionIssueLevelAllAfterCodeError {
	return &PipelineOptionIssueLevelAllAfterCodeError{
		pipelineOptionError: pipelineOptionError{option: PipelineOptionAllIssueLevels},
	}
}

func (e *PipelineOptionIssueLevelAllAfterCodeError) Error() string {
	return fmt.Sprintf("pipeline option %s must occur before specific code settings", e.Option())
}

// PipelineOptionIssueLevelDuplicateCodeError reports an issue code with more than one level setting.
type PipelineOptionIssueLevelDuplicateCodeError struct {
	pipelineOptionError
	code issue.Code
}

func newPipelineOptionIssueLevelDuplicateCodeError(code issue.Code) *PipelineOptionIssueLevelDuplicateCodeError {
	return &PipelineOptionIssueLevelDuplicateCodeError{
		pipelineOptionError: pipelineOptionError{option: PipelineOptionIssueLevel},
		code:                code,
	}
}

// Code returns the issue code configured more than once.
func (e *PipelineOptionIssueLevelDuplicateCodeError) Code() issue.Code {
	return e.code
}

func (e *PipelineOptionIssueLevelDuplicateCodeError) Error() string {
	return fmt.Sprintf("pipeline option %s may specify code %q only once", e.Option(), e.Code())
}

// PipelineOptionValidationLevelAllAfterCodeError reports an all-code validation level configured after a specific
// validation code.
type PipelineOptionValidationLevelAllAfterCodeError struct {
	pipelineOptionError
}

func newPipelineOptionValidationLevelAllAfterCodeError() *PipelineOptionValidationLevelAllAfterCodeError {
	return &PipelineOptionValidationLevelAllAfterCodeError{
		pipelineOptionError: pipelineOptionError{option: PipelineOptionAllValidationLevels},
	}
}

func (e *PipelineOptionValidationLevelAllAfterCodeError) Error() string {
	return fmt.Sprintf("pipeline option %s must occur before specific code settings", e.Option())
}

// PipelineOptionValidationLevelDuplicateCodeError reports a validation code with more than one level setting.
type PipelineOptionValidationLevelDuplicateCodeError struct {
	pipelineOptionError
	code validation.Code
}

func newPipelineOptionValidationLevelDuplicateCodeError(
	code validation.Code,
) *PipelineOptionValidationLevelDuplicateCodeError {
	return &PipelineOptionValidationLevelDuplicateCodeError{
		pipelineOptionError: pipelineOptionError{option: PipelineOptionValidationLevel},
		code:                code,
	}
}

// Code returns the validation code configured more than once.
func (e *PipelineOptionValidationLevelDuplicateCodeError) Code() validation.Code {
	return e.code
}

func (e *PipelineOptionValidationLevelDuplicateCodeError) Error() string {
	return fmt.Sprintf("pipeline option %s may specify code %q only once", e.Option(), e.Code())
}
