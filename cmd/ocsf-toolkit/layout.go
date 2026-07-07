package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
)

// inputEvent identifies an event source and, for directory input, its path relative to the
// selected input root. It retains layout information; eventFileLayout adds filesystem identity
// and resolved output destinations.
type inputEvent struct {
	path string
	rel  string
}

// filesystemPath keeps the representations needed to reason about one filesystem location.
// display is the user-supplied or calculated path shown in output, absolute is its cleaned
// absolute form without resolving symlinks, and resolved follows symlinks in the existing
// prefix while retaining any suffix that does not exist yet. caseInsensitive records how
// the filesystem containing that existing prefix compares path names.
type filesystemPath struct {
	display         string
	absolute        string
	resolved        string
	caseInsensitive bool
}

// outputDestination associates an enabled output with its filesystem identity. A nil
// destination means that output is disabled; a destination whose display path is "-"
// writes to stdout and has empty absolute and resolved forms.
type outputDestination struct {
	path filesystemPath
}

// eventFileLayout binds an input to every output selected for that event. It prevents runtime
// path validation and writing from independently regenerating or normalizing destinations.
type eventFileLayout struct {
	input            inputEvent
	inputPath        *filesystemPath
	eventOutput      *outputDestination
	validationOutput *outputDestination
	unenrichIssues   *outputDestination
}

// processingLayout is the immutable filesystem layout for one command. Path safety and
// collisions are checked while building it; processing and summary writing consume the
// same destinations afterward.
type processingLayout struct {
	events            []eventFileLayout
	summaryOutput     *outputDestination
	summaryJSONOutput *outputDestination
}

func newFilesystemPath(display string) (filesystemPath, error) {
	absolute, err := filepath.Abs(display)
	if err != nil {
		return filesystemPath{}, fmt.Errorf("resolve absolute path for %q: %w", display, err)
	}
	absolute = filepath.Clean(absolute)
	resolved, existingPrefix, err := resolveExistingPathPrefix(absolute)
	if err != nil {
		return filesystemPath{}, fmt.Errorf("resolve symlinks for %q: %w", display, err)
	}
	return filesystemPath{
		display:         display,
		absolute:        absolute,
		resolved:        resolved,
		caseInsensitive: pathLookupIsCaseInsensitive(existingPrefix),
	}, nil
}

func resolveExistingPathPrefix(absolute string) (string, string, error) {
	current := absolute
	missing := make([]string, 0)
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolvedPrefix, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", "", err
			}
			resolved := resolvedPrefix
			for _, component := range slices.Backward(missing) {
				resolved = filepath.Join(resolved, component)
			}
			return filepath.Clean(resolved), filepath.Clean(resolvedPrefix), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", "", fmt.Errorf("no existing ancestor for %q", absolute)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func pathLookupIsCaseInsensitive(existingPath string) bool {
	for current := existingPath; ; current = filepath.Dir(current) {
		parent := filepath.Dir(current)
		if parent == current {
			return defaultPathLookupIsCaseInsensitive()
		}
		alternate, changed := togglePathNameCase(filepath.Base(current))
		if !changed {
			continue
		}
		info, err := os.Stat(current)
		if err != nil {
			return defaultPathLookupIsCaseInsensitive()
		}
		alternateInfo, err := os.Stat(filepath.Join(parent, alternate))
		if err == nil {
			return os.SameFile(info, alternateInfo)
		}
		if errors.Is(err, fs.ErrNotExist) {
			return false
		}
		return defaultPathLookupIsCaseInsensitive()
	}
}

func defaultPathLookupIsCaseInsensitive() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

func togglePathNameCase(name string) (string, bool) {
	for index, char := range name {
		switch {
		case char >= 'a' && char <= 'z':
			return name[:index] + string(char-'a'+'A') + name[index+1:], true
		case char >= 'A' && char <= 'Z':
			return name[:index] + string(char-'A'+'a') + name[index+1:], true
		}
	}
	return name, false
}

func newOutputDestination(display string) (*outputDestination, error) {
	if display == stdioPath {
		return &outputDestination{path: filesystemPath{display: display}}, nil
	}
	path, err := newFilesystemPath(display)
	if err != nil {
		return nil, err
	}
	return &outputDestination{path: path}, nil
}

func pathIdentity(path filesystemPath) string {
	if path.caseInsensitive {
		return strings.ToUpper(path.resolved)
	}
	return path.resolved
}

func pathContains(root, path filesystemPath) bool {
	rootResolved := root.resolved
	pathResolved := path.resolved
	if root.caseInsensitive || path.caseInsensitive {
		rootResolved = strings.ToUpper(rootResolved)
		pathResolved = strings.ToUpper(pathResolved)
	}
	relative, err := filepath.Rel(rootResolved, pathResolved)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func pathsOverlap(left, right filesystemPath) bool {
	return pathContains(left, right) || pathContains(right, left)
}

func buildProcessingLayout(config processConfig) (processingLayout, error) {
	var outputRoot *filesystemPath
	if config.outputDir != "" {
		path, err := newFilesystemPath(config.outputDir)
		if err != nil {
			return processingLayout{}, fmt.Errorf("resolve output directory: %w", err)
		}
		outputRoot = &path
	}
	if config.eventsDir != "" && outputRoot != nil {
		inputRoot, err := newFilesystemPath(config.eventsDir)
		if err != nil {
			return processingLayout{}, fmt.Errorf("resolve events input directory: %w", err)
		}
		if pathsOverlap(inputRoot, *outputRoot) {
			return processingLayout{}, errors.New("input and output directory trees must not overlap")
		}
	}

	inputs, err := collectInputs(config)
	if err != nil {
		return processingLayout{}, err
	}
	layout := processingLayout{events: make([]eventFileLayout, 0, len(inputs))}
	reserved := make(map[string]string)
	for _, input := range inputs {
		eventLayout := eventFileLayout{input: input}
		if input.path != stdioPath {
			path, err := newFilesystemPath(input.path)
			if err != nil {
				return processingLayout{}, fmt.Errorf("resolve input event %q: %w", input.path, err)
			}
			eventLayout.inputPath = &path
			if err := reserveLayoutPath(reserved, path, fmt.Sprintf("input event %q", input.path)); err != nil {
				return processingLayout{}, err
			}
		}

		if config.mutatesEvent() {
			eventLayout.eventOutput, err = resolveOutputDestination(
				outputRoot, eventOutputPath(config, input), "processed event", input.path, reserved, !config.updateInPlace,
			)
			if err != nil {
				return processingLayout{}, err
			}
		}
		if config.validate {
			eventLayout.validationOutput, err = resolveOutputDestination(
				outputRoot, validationOutputPath(config, input), "validation report", input.path, reserved, true,
			)
			if err != nil {
				return processingLayout{}, err
			}
		}
		if config.unenrich {
			eventLayout.unenrichIssues, err = resolveOutputDestination(
				outputRoot, unenrichIssuesOutputPath(config, input), "enrichment-removal report", input.path, reserved, true,
			)
			if err != nil {
				return processingLayout{}, err
			}
		}
		layout.events = append(layout.events, eventLayout)
	}

	if config.summaryOutput != "" {
		layout.summaryOutput, err = resolveOutputDestination(
			nil, config.summaryOutput, "human-readable summary", "", reserved, true,
		)
		if err != nil {
			return processingLayout{}, err
		}
	}
	if config.summaryJSONOutput != "" {
		layout.summaryJSONOutput, err = resolveOutputDestination(
			nil, config.summaryJSONOutput, "JSON summary", "", reserved, true,
		)
		if err != nil {
			return processingLayout{}, err
		}
	}
	return layout, nil
}

func resolveOutputDestination(
	outputRoot *filesystemPath,
	display string,
	kind string,
	input string,
	reserved map[string]string,
	reserve bool,
) (*outputDestination, error) {
	destination, err := newOutputDestination(display)
	if err != nil {
		return nil, fmt.Errorf("resolve %s path %q: %w", kind, display, err)
	}
	if destination.path.display == stdioPath {
		return destination, nil
	}
	if outputRoot != nil {
		if err := validatePathBeneathOutputRoot(*outputRoot, destination.path); err != nil {
			return nil, err
		}
	}
	if !reserve {
		return destination, nil
	}
	description := kind
	if input != "" {
		description = fmt.Sprintf("%s for input %q", kind, input)
	}
	if err := reserveLayoutPath(reserved, destination.path, description); err != nil {
		return nil, err
	}
	return destination, nil
}

func reserveLayoutPath(paths map[string]string, path filesystemPath, description string) error {
	identity := pathIdentity(path)
	if previous, collision := paths[identity]; collision {
		return fmt.Errorf("path %q is selected for both %s and %s", path.display, previous, description)
	}
	paths[identity] = description
	return nil
}

func validatePathBeneathOutputRoot(root, path filesystemPath) error {
	relative, err := filepath.Rel(root.absolute, path.absolute)
	if err != nil {
		return fmt.Errorf("failed to resolve output path %q relative to root %q: %w", path.display, root.display, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return fmt.Errorf("output path %q escapes output root %q", path.display, root.display)
	}
	current := root.absolute
	directory := filepath.Dir(relative)
	if directory != "." {
		for component := range strings.SplitSeq(directory, string(filepath.Separator)) {
			current = filepath.Join(current, component)
			info, err := os.Lstat(current)
			if errors.Is(err, fs.ErrNotExist) {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to inspect output directory %q: %w", current, err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf(
					"output path %q traverses symbolic link %q beneath output root %q",
					path.display,
					current,
					root.display,
				)
			}
			if !info.IsDir() {
				return fmt.Errorf("output path %q has non-directory parent %q", path.display, current)
			}
		}
	}
	if !pathContains(root, path) {
		return fmt.Errorf("output path %q resolves outside output root %q", path.display, root.display)
	}
	return nil
}

func collectInputs(config processConfig) ([]inputEvent, error) {
	if config.eventPath != "" {
		if config.eventPath == stdioPath {
			return []inputEvent{{path: stdioPath, rel: stdinEventRelativePath}}, nil
		}
		return []inputEvent{{path: config.eventPath}}, nil
	}

	inputs := make([]inputEvent, 0)
	err := filepath.WalkDir(config.eventsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(path), ".json") {
			return nil
		}
		rel, err := filepath.Rel(config.eventsDir, path)
		if err != nil {
			return err
		}
		inputs = append(inputs, inputEvent{path: path, rel: rel})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk events directory %q: %w", config.eventsDir, err)
	}
	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].rel < inputs[j].rel
	})
	return inputs, nil
}
func eventOutputPath(config processConfig, input inputEvent) string {
	if config.updateInPlace {
		return input.path
	}
	if config.eventPath != "" && config.eventOutput != "" {
		return config.eventOutput
	}
	return filepath.Join(config.outputDir, eventOutputRelativePath(input))
}
func validationOutputPath(config processConfig, input inputEvent) string {
	if config.eventPath != "" && config.validationOutput != "" {
		return config.validationOutput
	}
	if config.outputDir == "" {
		return ""
	}
	return filepath.Join(config.outputDir, validationRelativePath(eventOutputRelativePath(input)))
}
func unenrichIssuesOutputPath(config processConfig, input inputEvent) string {
	if config.eventPath != "" && config.unenrichIssuesOutput != "" {
		return config.unenrichIssuesOutput
	}
	if config.outputDir == "" {
		return ""
	}
	return filepath.Join(config.outputDir, unenrichIssuesRelativePath(eventOutputRelativePath(input)))
}

func eventOutputRelativePath(input inputEvent) string {
	if input.rel != "" {
		return safeOutputRelativePath(input.rel)
	}
	if input.path != stdioPath && !filepath.IsAbs(input.path) {
		return safeOutputRelativePath(input.path)
	}
	return filepath.Base(input.path)
}

func safeOutputRelativePath(path string) string {
	cleanPath := filepath.Clean(path)
	if slices.Contains(strings.Split(cleanPath, string(filepath.Separator)), "..") {
		return filepath.Base(cleanPath)
	}
	return cleanPath
}

func validationRelativePath(inputRel string) string {
	return reportRelativePath(inputRel, "-validation.json")
}

func unenrichIssuesRelativePath(inputRel string) string {
	return reportRelativePath(inputRel, "-unenrich-issues.json")
}

func reportRelativePath(inputRel, suffix string) string {
	dir := filepath.Dir(inputRel)
	base := filepath.Base(inputRel)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	reportName := name + suffix
	if dir == "." {
		return reportName
	}
	return filepath.Join(dir, reportName)
}
