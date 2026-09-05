package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/validation"
)

type cliOptions struct {
	General             generalOptions
	Mutation            mutationOptions
	Issues              issueOptions
	Validation          validationOptions
	Help                bool
	ListIssueCodes      bool
	ListValidationCodes bool
}

type generalOptions struct {
	Schema                 string
	Event                  string
	EventsDir              string
	OutputDir              string
	EventOutput            string
	ReportOutput           string
	SummaryFile            string
	SummaryJSONFile        string
	ObservablePathNotation string
	Overwrite              bool
	PrettyJSON             bool
	Quiet                  bool
	Version                bool
}

type validationOptions struct {
	Validate               bool
	FailOnValidationErrors bool
	Levels                 validationLevelsOption
}

type issueOptions struct {
	Levels issueLevelsOption
}

type mutationOptions struct {
	Enrich                  bool
	Unenrich                bool
	ForceRemove             bool
	EnumSiblings            enrichmentActionOption
	Observables             enrichmentActionOption
	ObservableIDs           observableTypeIDsOption
	ObservableDeduplication observableDeduplicationOption
}

type issueLevelsOption struct {
	optionName string
	rules      []issueLevelRule
}

type issueLevelRule struct {
	code  issue.Code
	level issue.Level
	all   bool
}

func (o *issueLevelsOption) setOptionName(name string) {
	o.optionName = name
}

//nolint:dupl // Typed issue and validation parsing keeps package-specific errors clear.
func (o *issueLevelsOption) Set(value string) error {
	codeName, levelName, ok := strings.Cut(value, "=")
	if !ok || codeName == "" || levelName == "" {
		return fmt.Errorf("invalid value %q for %s: expected ISSUE_CODE=LEVEL", value, o.optionName)
	}
	level, ok := issue.ParseLevel(levelName)
	if !ok {
		return fmt.Errorf("unknown issue level %q in %s", levelName, o.optionName)
	}
	if codeName == "all" {
		o.rules = append(o.rules, issueLevelRule{level: level, all: true})
		return nil
	}
	code, ok := issue.ParseCode(codeName)
	if !ok {
		return fmt.Errorf("unknown issue code %q in %s", codeName, o.optionName)
	}
	o.rules = append(o.rules, issueLevelRule{code: code, level: level})
	return nil
}

func (*issueLevelsOption) String() string {
	return ""
}

func (*issueLevelsOption) Type() string {
	return "issue level"
}

type validationLevelsOption struct {
	optionName string
	rules      []validationLevelRule
}

type validationLevelRule struct {
	code  validation.Code
	level validation.Level
	all   bool
}

func (o *validationLevelsOption) setOptionName(name string) {
	o.optionName = name
}

//nolint:dupl // Typed issue and validation parsing keeps package-specific errors clear.
func (o *validationLevelsOption) Set(value string) error {
	codeName, levelName, ok := strings.Cut(value, "=")
	if !ok || codeName == "" || levelName == "" {
		return fmt.Errorf("invalid value %q for %s: expected VALIDATION_CODE=LEVEL", value, o.optionName)
	}
	level, ok := validation.ParseLevel(levelName)
	if !ok {
		return fmt.Errorf("unknown validation level %q in %s", levelName, o.optionName)
	}
	if codeName == "all" {
		o.rules = append(o.rules, validationLevelRule{level: level, all: true})
		return nil
	}
	code, ok := validation.ParseCode(codeName)
	if !ok {
		return fmt.Errorf("unknown validation code %q in %s", codeName, o.optionName)
	}
	o.rules = append(o.rules, validationLevelRule{code: code, level: level})
	return nil
}

func (*validationLevelsOption) String() string {
	return ""
}

func (*validationLevelsOption) Type() string {
	return "validation level"
}

// setOnceOption guards a pflag.Value option against being specified more than once, and names itself as optionName
// (set automatically by cliFlagRegistration from the flag's own name) in error messages.
type setOnceOption struct {
	optionName string
	configured bool
}

func (o *setOnceOption) setOptionName(name string) {
	o.optionName = name
}

func (o *setOnceOption) markConfigured() error {
	if o.configured {
		return fmt.Errorf("%s may only be specified once", o.optionName)
	}
	o.configured = true
	return nil
}

// namedOption lets cliFlagRegistration tell an option value its own flag name, so Set's error messages can name the
// flag without duplicating that name as a literal string at each call site.
type namedOption interface {
	setOptionName(name string)
}

type enrichmentActionOption struct {
	setOnceOption
	action enrichment.Action
}

func (o *enrichmentActionOption) Set(value string) error {
	if err := o.markConfigured(); err != nil {
		return err
	}
	action := enrichment.Action(value)
	if !action.Valid() || action == enrichment.None {
		return fmt.Errorf("invalid value %q for %s: expected add, remove, or force-remove", value, o.optionName)
	}
	o.action = action
	return nil
}

func (o *enrichmentActionOption) String() string {
	return string(o.action)
}

func (*enrichmentActionOption) Type() string {
	return "action"
}

type observableDeduplicationOption struct {
	setOnceOption
	mode enrichment.ObservableDeduplication
}

func (o *observableDeduplicationOption) Set(value string) error {
	if err := o.markConfigured(); err != nil {
		return err
	}
	switch value {
	case "disabled":
		o.mode = enrichment.ObservableDeduplicationIgnored
	case "generated":
		o.mode = enrichment.ObservableDeduplicationGenerated
	default:
		return fmt.Errorf("invalid value %q for %s: expected disabled or generated", value, o.optionName)
	}
	return nil
}

func (o *observableDeduplicationOption) String() string {
	if o.mode == enrichment.ObservableDeduplicationIgnored {
		return "disabled"
	}
	return string(o.mode)
}

func (*observableDeduplicationOption) Type() string {
	return "observable deduplication"
}

type observableTypeIDsOption struct {
	optionName string
	configured bool
	values     []int64
}

func (o *observableTypeIDsOption) setOptionName(name string) {
	o.optionName = name
}

func (o *observableTypeIDsOption) Set(value string) error {
	if value == "" {
		return fmt.Errorf("%s requires an observable type ID", o.optionName)
	}
	typeID, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf(
			"invalid observable type ID %q in %s: must be a signed 64-bit integer", value, o.optionName,
		)
	}
	o.configured = true
	o.values = append(o.values, typeID)
	return nil
}

func (o *observableTypeIDsOption) String() string {
	values := make([]string, len(o.values))
	for index, value := range o.values {
		values[index] = strconv.FormatInt(value, 10)
	}
	return strings.Join(values, ",")
}

func (*observableTypeIDsOption) Type() string {
	return "observable ID"
}
