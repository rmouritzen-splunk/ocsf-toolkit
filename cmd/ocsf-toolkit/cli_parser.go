package main

import (
	"io"
	"strings"

	"github.com/ocsf/ocsf-toolkit/enrichment"
	"github.com/spf13/pflag"
)

type cliParser struct {
	flags  *pflag.FlagSet
	groups []cliFlagGroup
}

type cliFlagGroup struct {
	name  string
	flags []*pflag.Flag
}

type cliFlagRegistration struct {
	parser *cliParser
	group  *cliFlagGroup
}

func (p *cliParser) parse(args []string) error {
	return p.flags.Parse(normalizeOptionalActionValues(args))
}

// pflag treats a flag with NoOptDefVal as bare even when the next argument is a value. Normalize the two optional
// action flags so their canonical --flag value spelling and their attached --flag=value spelling behave identically.
func normalizeOptionalActionValues(args []string) []string {
	normalized := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if (arg == "--enum-siblings" || arg == "--observables") && index+1 < len(args) &&
			!strings.HasPrefix(args[index+1], "-") {
			normalized = append(normalized, arg+"="+args[index+1])
			index++
			continue
		}
		normalized = append(normalized, arg)
	}
	return normalized
}

const (
	flagValueNameAnnotation = "ocsf-toolkit/value-name"
)

func newParser() (*cliParser, *cliOptions) {
	options := &cliOptions{}
	options.Mutation.ObservableDeduplication.mode = enrichment.ObservableDeduplicationIgnored
	parser := &cliParser{flags: pflag.NewFlagSet("ocsf-toolkit", pflag.ContinueOnError)}
	parser.flags.SetOutput(io.Discard)
	parser.flags.SortFlags = false

	general := parser.newGroup("General Options")
	general.stringVarP(&options.General.Schema, "schema", "s", "COMPILED_SCHEMA_FILE", "Compiled OCSF schema file")
	general.stringVarP(&options.General.Event, "event", "e", "FILE", "Single event JSON file, or - for stdin")
	general.stringVarP(&options.General.EventsDir, "events-dir", "d", "DIR", "Directory tree of event JSON files")
	general.stringVarP(
		&options.General.OutputDir,
		"output-dir",
		"o",
		"DIR",
		`Output root containing subdirectories named "events" and "reports"`,
	)
	general.stringVar(
		&options.General.EventOutput,
		"event-output",
		"FILE",
		"Single processed event output file, or - for stdout",
	)
	general.stringVar(
		&options.General.ReportOutput,
		"report-output",
		"FILE",
		"Single processing report output file, or - for stdout",
	)
	general.stringVar(
		&options.General.SummaryFile,
		"summary",
		"FILE",
		"Human-readable directory summary file, or - for stdout",
	)
	general.stringVar(
		&options.General.SummaryJSONFile,
		"summary-json",
		"FILE",
		"JSON directory summary file, or - for stdout",
	)
	general.stringVar(
		&options.General.ObservablePathNotation,
		"observable-path-notation",
		"STYLE",
		"Notation for generated observable names and validation findings:"+
			" simple, brackets, wildcard, indexed, or jsonpath",
	)
	general.boolVar(&options.General.Overwrite, "overwrite", "", "Allow replacing existing output files")
	general.boolVar(&options.General.PrettyJSON, "pretty-json", "p", "Pretty-print JSON output, including stdout")
	general.boolVar(&options.General.Quiet, "quiet", "q", "Suppress the default directory summary on stdout")
	general.boolVar(&options.General.Version, "version", "v", "Print version information and exit")

	mutation := parser.newGroup("Enrichment Options")
	mutation.actionVar(
		&options.Mutation.EnumSiblings,
		"enum-siblings",
		"Set the enum sibling action: add, remove, or force-remove; defaults to add",
	)
	mutation.actionVar(
		&options.Mutation.Observables,
		"observables",
		"Set the observable action: add, remove, or force-remove; defaults to add",
	)
	mutation.varValue(
		&options.Mutation.ObservableIDs,
		"observable-id",
		"ID",
		"Add only the observable type ID; may be repeated",
	)
	mutation.varValue(
		&options.Mutation.ObservableDeduplication,
		"deduplicate-observables",
		"MODE",
		"Deduplicate observables: disabled or generated; defaults to disabled."+
			" Generated mode compares generated candidates only; existing observables do not suppress generation",
	)
	mutation.boolVar(
		&options.Mutation.Enrich,
		"enrich",
		"E",
		"Add enum siblings and observables (shorthand for --enum-siblings add --observables add)",
	)

	mutation.boolVar(
		&options.Mutation.Unenrich,
		"unenrich",
		"U",
		"Safely remove enum siblings and observables (shorthand for --enum-siblings remove --observables remove)",
	)
	mutation.boolVar(
		&options.Mutation.ForceRemove,
		"force-remove",
		"",
		"Force-remove enum siblings and observables"+
			" (shorthand for --enum-siblings force-remove --observables force-remove)",
	)

	issues := parser.newGroup("Issue Options")
	issues.varValue(
		&options.Issues.Levels,
		"issue-level",
		"ISSUE_CODE=LEVEL",
		"Set an issue level to ignored, warning, or error; repeat for additional codes, or set all=LEVEL",
	)

	validationGroup := parser.newGroup("Validation Options")
	validationGroup.boolVar(&options.Validation.Validate, "validate", "V", "Validate events")
	validationGroup.boolVar(
		&options.Validation.FailOnValidationErrors,
		"fail-on-validation-errors",
		"",
		"Exit non-zero when validation errors are found",
	)
	validationGroup.varValue(
		&options.Validation.Levels,
		"validation-level",
		"VALIDATION_CODE=LEVEL",
		"Set a validation level to ignored, warning, or error; repeat for additional codes, or set all=LEVEL",
	)

	help := parser.newGroup("Help Options")
	help.boolVar(&options.Help, "help", "h", "Show this help message")
	help.boolVar(&options.ListIssueCodes, "list-issue-codes", "", "List all issue codes and exit")
	help.boolVar(&options.ListValidationCodes, "list-validation-codes", "", "List all validation codes and exit")
	return parser, options
}

func (p *cliParser) newGroup(name string) cliFlagRegistration {
	p.groups = append(p.groups, cliFlagGroup{name: name})
	return cliFlagRegistration{parser: p, group: &p.groups[len(p.groups)-1]}
}

func (r cliFlagRegistration) add(name, valueName string) *pflag.Flag {
	flag := r.parser.flags.Lookup(name)
	if valueName != "" {
		flag.Annotations = map[string][]string{flagValueNameAnnotation: {valueName}}
	}
	r.group.flags = append(r.group.flags, flag)
	return flag
}

func (r cliFlagRegistration) stringVarP(target *string, name, shorthand, valueName, usage string) {
	r.parser.flags.StringVarP(target, name, shorthand, "", usage)
	r.add(name, valueName)
}

func (r cliFlagRegistration) stringVar(target *string, name, valueName, usage string) {
	r.stringVarP(target, name, "", valueName, usage)
}

func (r cliFlagRegistration) boolVar(target *bool, name, shorthand, usage string) {
	r.parser.flags.BoolVarP(target, name, shorthand, false, usage)
	r.add(name, "")
}

func (r cliFlagRegistration) varValue(target pflag.Value, name, valueName, usage string) {
	nameOption(target, name)
	r.parser.flags.Var(target, name, usage)
	r.add(name, valueName)
}

// nameOption tells target its own flag name (as "--name"), if it implements namedOption, so its Set method's error
// messages can name the flag without each call site duplicating that name as a literal string.
func nameOption(target pflag.Value, name string) {
	if named, ok := target.(namedOption); ok {
		named.setOptionName("--" + name)
	}
}

func (r cliFlagRegistration) actionVar(
	target *enrichmentActionOption,
	name string,
	usage string,
) {
	nameOption(target, name)
	r.parser.flags.Var(target, name, usage)
	flag := r.add(name, "[ACTION]")
	flag.NoOptDefVal = string(enrichment.Add)
}
