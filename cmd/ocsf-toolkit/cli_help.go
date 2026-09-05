package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ocsf/ocsf-toolkit/issue"
	"github.com/ocsf/ocsf-toolkit/validation"
	"github.com/spf13/pflag"
	"golang.org/x/term"
)

// helpEntryIndent is the indent given to each top-level help entry (list-issue-codes entries, Notes items),
// matching the flag-option indent under a group header; nested detail lines get one further level.
const helpEntryIndent = 2

// formatHelpEntry renders primary at helpEntryIndent, followed by each detail indented one further level, as a
// single block ready to be joined with other entries by joinHelpEntries.
func formatHelpEntry(primary string, details ...string) string {
	lines := make([]string, 0, 1+len(details))
	lines = append(lines, strings.Repeat(" ", helpEntryIndent)+primary)
	for _, detail := range details {
		lines = append(lines, strings.Repeat(" ", helpEntryIndent*2)+detail)
	}
	return strings.Join(lines, "\n")
}

// joinHelpEntries joins entries with a blank line between each, for use with writeWrappedLines.
func joinHelpEntries(entries []string) string {
	return strings.Join(entries, "\n\n")
}

// joinHelpSection joins a header with its entries, separated by a blank line, for use with writeWrappedLines.
func joinHelpSection(header string, entries []string) string {
	return header + "\n\n" + joinHelpEntries(entries)
}

// writeIssueCodes lists every issue code and its description, sorted, one blank-line-separated entry per code,
// annotating mandatory codes that cannot use the ignored level.
func writeIssueCodes(w io.Writer, width int) {
	codes := issue.Codes()
	sort.Slice(codes, func(i, j int) bool { return codes[i].String() < codes[j].String() })

	entries := make([]string, len(codes))
	for i, code := range codes {
		name := fmt.Sprintf("%s (default: %s)", code, code.DefaultLevel())
		if !code.Ignorable() {
			name += " (mandatory, cannot be ignored)"
		}
		entries[i] = formatHelpEntry(name, code.Description())
	}
	header := "Issue codes:"
	writeWrappedLines(w, joinHelpSection(header, entries), width)
}

func writeValidationCodes(w io.Writer, width int) {
	codes := validation.Codes()
	sort.Slice(codes, func(i, j int) bool { return codes[i].String() < codes[j].String() })

	entries := make([]string, len(codes))
	for index, code := range codes {
		name := fmt.Sprintf("%s (default: %s)", code, code.DefaultLevel())
		entries[index] = formatHelpEntry(name, code.Description())
	}
	header := "Validation codes:"
	writeWrappedLines(w, joinHelpSection(header, entries), width)
}

func writeErrorUsage(w io.Writer, parser *cliParser) {
	writef(w, "Usage:\n")
	writef(w, "  %s %s\n", parser.flags.Name(), cliUsage)
	writef(w, "Run \"ocsf-toolkit --help\" for full usage.\n")
}

func writeHelp(w io.Writer, parser *cliParser) {
	width := helpOutputWidth(w)
	writef(w, "Usage:\n  %s %s\n\n", parser.flags.Name(), cliUsage)
	writeWrapped(
		w,
		"Process OCSF event files with enrichment, enrichment removal, and validation using a compiled OCSF schema."+
			" Select at least one processing action; compatible actions may be combined.",
		0,
		width,
	)
	writef(w, "\n")
	for _, group := range parser.groups {
		writef(w, "%s:\n", group.name)
		for _, flag := range group.flags {
			writeHelpFlag(w, flag, width)
		}
		writef(w, "\n")
	}
	writeWrappedLines(w, processHelpNotes(), width)
}

func writeHelpFlag(w io.Writer, flag *pflag.Flag, width int) {
	writeHelpFlagLine(w, flag, flagAnnotation(flag, flagValueNameAnnotation), flag.Usage, true, width)
}

func writeHelpFlagLine(w io.Writer, flag *pflag.Flag, valueName, usage string, includeShorthand bool, width int) {
	descriptionColumn := helpDescriptionColumn(width)
	option := "--" + flag.Name
	if includeShorthand && flag.Shorthand != "" {
		option = "-" + flag.Shorthand + ", " + option
	} else {
		option = "    " + option
	}
	if valueName != "" {
		option += " " + valueName
	}
	prefix := "  " + option
	optionWidth := utf8.RuneCountInString(prefix)
	if optionWidth >= descriptionColumn {
		writef(w, "%s\n%s", prefix, strings.Repeat(" ", descriptionColumn))
		writeWrapped(w, usage, descriptionColumn, width)
		return
	}
	writef(w, "%s%s", prefix, strings.Repeat(" ", descriptionColumn-optionWidth))
	writeWrapped(w, usage, descriptionColumn, width)
}

func helpDescriptionColumn(width int) int {
	const preferredColumn = 42
	if width >= preferredColumn+20 {
		return preferredColumn
	}
	return max(4, width/2)
}

type descriptorWriter interface {
	Fd() uintptr
}

func helpOutputWidth(w io.Writer) int {
	return detectHelpOutputWidth(w, term.IsTerminal, term.GetSize)
}

func detectHelpOutputWidth(
	w io.Writer,
	isTerminal func(fd int) bool,
	getSize func(fd int) (width, height int, err error),
) int {
	descriptor, ok := w.(descriptorWriter)
	if !ok {
		return defaultHelpWidth
	}
	fd := int(descriptor.Fd())
	if !isTerminal(fd) {
		return defaultHelpWidth
	}
	width, _, err := getSize(fd)
	if err != nil || width <= 0 {
		return defaultHelpWidth
	}
	return max(minimumHelpWidth, width-terminalHelpMargin)
}

func writeWrapped(w io.Writer, value string, indent, width int) {
	writeWrappedHanging(w, value, indent, indent, width)
}

func writeWrappedHanging(w io.Writer, value string, initialColumn, continuationIndent, width int) {
	words := strings.Fields(value)
	column := initialColumn
	lineHasWord := false
	for _, word := range words {
		separator := ""
		if lineHasWord {
			separator = " "
		}
		if lineHasWord && column+len(separator)+utf8.RuneCountInString(word) > width {
			writef(w, "\n%s", strings.Repeat(" ", continuationIndent))
			column = continuationIndent
			separator = ""
		}
		writef(w, "%s%s", separator, word)
		column += len(separator) + utf8.RuneCountInString(word)
		lineHasWord = true
	}
	writef(w, "\n")
}

func writeWrappedLines(w io.Writer, value string, width int) {
	for line := range strings.SplitSeq(value, "\n") {
		content := strings.TrimLeft(line, " ")
		if content == "" {
			writef(w, "\n")
			continue
		}
		indent := len(line) - len(content)
		continuationIndent := indent
		if strings.HasPrefix(content, "- ") {
			continuationIndent += 2
		}
		writef(w, "%s", strings.Repeat(" ", indent))
		writeWrappedHanging(w, content, indent, continuationIndent, width)
	}
}

func flagAnnotation(flag *pflag.Flag, name string) string {
	if values := flag.Annotations[name]; len(values) > 0 {
		return values[0]
	}
	return ""
}

func processHelpNotes() string {
	notes := []string{
		formatHelpEntry(
			"Flag values may be separated by a space or attached with =; for example:",
			"--event my_event.json",
			"--event=my_event.json",
		),
		formatHelpEntry("Only one output option may use stdout."),
		formatHelpEntry(
			"--output-dir writes processed events beneath events/ and processing reports beneath reports/.",
		),
		formatHelpEntry("--events-dir must name an existing directory, not a symbolic link.",
			"Symbolic links within the directory tree are ignored.",
		),
		formatHelpEntry("Both output subdirectories preserve input-relative paths.",
			"With --events-dir, paths are relative to that directory.",
			"With --event, relative paths are cleaned and preserved;"+
				" absolute paths and paths that would escape the current directory using .. use a safe basename.",
		),
		formatHelpEntry("Enum sibling work always runs before observable work, regardless of flag order."),
		formatHelpEntry("Forced observable removal deletes the entire observables attribute without inspecting it."),
		formatHelpEntry("Forced enum sibling removal retains siblings required for enum ID 99."),
	}
	return joinHelpSection("Notes:", notes)
}
