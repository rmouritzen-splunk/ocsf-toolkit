package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/ocsf/ocsf-toolkit/validation"
)

type cliOptions struct {
	General             generalOptions
	Mutation            mutationOptions
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
	Validate                 bool
	WarnOnMissingRecommended bool
	FailOnValidationErrors   bool
	Suppress                 validationCodesOption
	WarningsAsErrors         validationCodesOption
	ErrorsAsWarnings         validationCodesOption
}

type mutationOptions struct {
	Enrich         bool
	Unenrich       bool
	ForceRemove    bool
	EnumSiblings   enrichmentActionOption
	Observables    enrichmentActionOption
	ObservableIDs  observableTypeIDsOption
	SuppressIssues suppressIssuesOption
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

// isEmptyOrAllCodesValue reports whether value is the flag-omitted-entirely case (empty) or the flag-given-without-a-
// value case (optionalCodesAllValue, via NoOptDefVal): both mean "every code", so Set should record no explicit codes.
func isEmptyOrAllCodesValue(value string) bool {
	return value == "" || value == optionalCodesAllValue
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

type observableTypeIDsOption struct {
	setOnceOption
	values []int64
}

func (o *observableTypeIDsOption) Set(value string) error {
	if err := o.markConfigured(); err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("%s requires at least one observable type ID", o.optionName)
	}
	components := strings.Split(value, ",")
	o.values = make([]int64, len(components))
	for index, component := range components {
		if component == "" {
			return fmt.Errorf("%s contains an empty observable type ID", o.optionName)
		}
		typeID, err := strconv.ParseInt(component, 10, 64)
		if err != nil {
			return fmt.Errorf(
				"invalid observable type ID %q in %s: must be a signed 64-bit integer", component, o.optionName,
			)
		}
		o.values[index] = typeID
	}
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
	return "observable IDs"
}

// optionalCodesAllValue is the NoOptDefVal used when a code-selection option is given without a value. It is not a
// valid issue or validation code, so it cannot collide with a real one.
const optionalCodesAllValue = "*"

type suppressIssuesOption struct {
	setOnceOption
	values []string
}

func (o *suppressIssuesOption) Set(value string) error {
	if err := o.markConfigured(); err != nil {
		return err
	}
	if isEmptyOrAllCodesValue(value) {
		return nil
	}
	o.values = strings.Split(value, ",")
	return nil
}

func (o *suppressIssuesOption) String() string {
	return strings.Join(o.values, ",")
}

func (*suppressIssuesOption) Type() string {
	return "issue codes"
}

type validationCodesOption struct {
	setOnceOption
	codes []validation.Code
}

func (o *validationCodesOption) Set(value string) error {
	if err := o.markConfigured(); err != nil {
		return err
	}
	if isEmptyOrAllCodesValue(value) {
		return nil
	}
	for component := range strings.SplitSeq(value, ",") {
		if component == "" {
			return fmt.Errorf("%s contains an empty validation code", o.optionName)
		}
		code, ok := validation.ParseCode(component)
		if !ok {
			return fmt.Errorf("unknown validation code %q in %s", component, o.optionName)
		}
		o.codes = append(o.codes, code)
	}
	return nil
}

func (o *validationCodesOption) String() string {
	values := make([]string, len(o.codes))
	for index, code := range o.codes {
		values[index] = code.String()
	}
	return strings.Join(values, ",")
}

func (*validationCodesOption) Type() string {
	return "validation codes"
}
