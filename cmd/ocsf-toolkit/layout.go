package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ocsf/ocsf-toolkit/internal/fserror"
)

const (
	eventsOutputDirectory  = "events"
	reportsOutputDirectory = "reports"
)

type inputEvent struct {
	path string
	rel  string
}

// singleEventInput builds the inputEvent for single-event mode (--event), which both
// buildProcessingDestinations (reserving output paths) and processEvents (writing them) must
// derive identically.
func singleEventInput(config processConfig) inputEvent {
	input := inputEvent{path: config.eventPath}
	if config.eventPath == stdioPath {
		input.rel = stdinEventRelativePath
	}
	return input
}

// filesystemPath retains the absolute spelling of a path and the form obtained by resolving
// symbolic links in its longest existing prefix.
type filesystemPath struct {
	display  string
	absolute string
	resolved string
}

type reservedPath struct {
	path        filesystemPath
	description string
}

type outputDestination struct {
	path filesystemPath
}

// processingDestinations contains command-wide output choices. Directory event destinations
// are derived lazily from each path supplied by filepath.WalkDir.
type processingDestinations struct {
	outputRoot      *filesystemPath
	eventOutput     *outputDestination
	reportOutput    *outputDestination
	summaryFiles    []*outputDestination
	summaryJSONFile *outputDestination
}

func newFilesystemPath(display string) (filesystemPath, error) {
	absolute, err := filepath.Abs(display)
	if err != nil {
		return filesystemPath{}, fmt.Errorf("resolve absolute path for %q: %w", display, fserror.QuotePaths(err))
	}
	absolute = filepath.Clean(absolute)
	resolved, err := resolveExistingPathPrefix(absolute)
	if err != nil {
		return filesystemPath{}, fmt.Errorf("resolve symlinks for %q: %w", display, err)
	}
	return filesystemPath{display: display, absolute: absolute, resolved: resolved}, nil
}

func resolveExistingPathPrefix(absolute string) (string, error) {
	current := absolute
	missing := make([]string, 0)
	for {
		_, err := os.Lstat(current)
		if err == nil {
			resolvedPrefix, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", fserror.QuotePaths(err)
			}
			resolved := resolvedPrefix
			for _, component := range slices.Backward(missing) {
				resolved = filepath.Join(resolved, component)
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fserror.QuotePaths(err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %q", absolute)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
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

func pathContains(root, path filesystemPath) bool {
	relative, err := filepath.Rel(root.resolved, path.resolved)
	if err == nil &&
		(relative == "." ||
			relative != ".." &&
				!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
				!filepath.IsAbs(relative)) {
		return true
	}
	return filesystemContainsPath(root.absolute, path.absolute)
}

// filesystemContainsPath is pathContains' filesystem-identity fallback. EvalSymlinks does not normalize
// case, so on a case-insensitive filesystem two differently-cased spellings of the same existing
// directory resolve to different strings even though they name the same location; comparing resolved
// strings alone would then miss a real overlap. This walks path's existing ancestors comparing
// filesystem identity (os.SameFile) against root, which catches that case regardless of spelling.
func filesystemContainsPath(root, path string) bool {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return false
	}
	current := path
	for {
		if info, err := os.Stat(current); err == nil && os.SameFile(rootInfo, info) {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

func sameFilesystemPath(left, right filesystemPath) bool {
	if left.resolved == right.resolved {
		return true
	}
	leftInfo, leftErr := os.Stat(left.absolute)
	rightInfo, rightErr := os.Stat(right.absolute)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

func pathsOverlap(left, right filesystemPath) bool {
	return pathContains(left, right) || pathContains(right, left)
}

func buildProcessingDestinations(config processConfig) (processingDestinations, error) {
	destinations, schemaPath, inputRoot, reserved, err := resolveDestinationRoots(config)
	if err != nil {
		return processingDestinations{}, err
	}
	if err := validateSchemaOutputNamespaces(config, destinations.outputRoot, schemaPath); err != nil {
		return processingDestinations{}, err
	}
	if err := resolveEventDestinations(config, &destinations, &reserved); err != nil {
		return processingDestinations{}, err
	}
	if err := resolveSummaryDestinations(config, &destinations, &reserved); err != nil {
		return processingDestinations{}, err
	}
	if err := validateSummaryDestinations(destinations, inputRoot); err != nil {
		return processingDestinations{}, err
	}
	return destinations, nil
}

func resolveDestinationRoots(
	config processConfig,
) (processingDestinations, filesystemPath, *filesystemPath, []reservedPath, error) {
	destinations := processingDestinations{}
	schemaPath, err := newFilesystemPath(config.schemaPath)
	if err != nil {
		return destinations, filesystemPath{}, nil, nil, fmt.Errorf("resolve schema %q: %w", config.schemaPath, err)
	}
	reserved := []reservedPath{{path: schemaPath, description: fmt.Sprintf("schema %q", config.schemaPath)}}
	if config.outputDir != "" {
		outputRoot, resolveErr := newFilesystemPath(config.outputDir)
		if resolveErr != nil {
			return destinations, filesystemPath{}, nil, nil, fmt.Errorf("resolve output directory: %w", resolveErr)
		}
		destinations.outputRoot = &outputRoot
		if err := validateOutputDirectory(config, outputRoot); err != nil {
			return destinations, filesystemPath{}, nil, nil, err
		}
	}
	var inputRoot *filesystemPath
	if config.eventsDir != "" && destinations.outputRoot != nil {
		root, resolveErr := newFilesystemPath(config.eventsDir)
		if resolveErr != nil {
			return destinations, filesystemPath{}, nil, nil,
				fmt.Errorf("resolve events input directory: %w", resolveErr)
		}
		if err := validateEventsDirectory(root); err != nil {
			return destinations, filesystemPath{}, nil, nil, err
		}
		inputRoot = &root
		if pathsOverlap(root, *destinations.outputRoot) {
			return destinations, filesystemPath{}, nil, nil,
				errors.New("input and output directory trees must not overlap")
		}
	}
	return destinations, schemaPath, inputRoot, reserved, nil
}

func validateSchemaOutputNamespaces(config processConfig, outputRoot *filesystemPath, schemaPath filesystemPath) error {
	if outputRoot == nil {
		return nil
	}
	for _, name := range activeOutputNamespaces(config) {
		if pathContains(outputNamespace(*outputRoot, name), schemaPath) {
			return fmt.Errorf(
				"schema path %q conflicts with reserved output namespace %q", config.schemaPath, name,
			)
		}
	}
	return nil
}

func resolveEventDestinations(
	config processConfig,
	destinations *processingDestinations,
	reserved *[]reservedPath,
) error {
	if config.eventPath != "" && config.eventPath != stdioPath {
		inputPath, err := newFilesystemPath(config.eventPath)
		if err != nil {
			return fmt.Errorf("resolve input event %q: %w", config.eventPath, err)
		}
		*reserved = append(*reserved, reservedPath{
			path: inputPath, description: fmt.Sprintf("input event %q", config.eventPath),
		})
	}
	if config.eventPath != "" {
		input := singleEventInput(config)
		if config.mutatesEvent() {
			var err error
			destinations.eventOutput, err = resolveDistinctDestination(
				eventOutputPath(config, input), "processed event", reserved,
			)
			if err != nil {
				return err
			}
		}
		if config.generatesReport() {
			var err error
			destinations.reportOutput, err = resolveDistinctDestination(
				reportOutputPath(config, input), "processing report", reserved,
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveSummaryDestinations(
	config processConfig,
	destinations *processingDestinations,
	reserved *[]reservedPath,
) error {
	if config.summaryFile != "" {
		summaryFile, resolveErr := resolveDistinctDestination(
			config.summaryFile, "human-readable summary", reserved,
		)
		if resolveErr != nil {
			return resolveErr
		}
		destinations.summaryFiles = append(destinations.summaryFiles, summaryFile)
	}
	if config.summaryJSONFile != "" {
		var err error
		destinations.summaryJSONFile, err = resolveDistinctDestination(
			config.summaryJSONFile, "JSON summary", reserved,
		)
		if err != nil {
			return err
		}
	}

	if config.eventsDir != "" && !config.quiet && config.summaryFile != stdioPath &&
		config.summaryJSONFile != stdioPath {
		destinations.summaryFiles = append(
			[]*outputDestination{displayDestination(stdioPath)},
			destinations.summaryFiles...,
		)
	}
	return nil
}

func validateSummaryDestinations(destinations processingDestinations, inputRoot *filesystemPath) error {
	if destinations.outputRoot != nil {
		for _, summary := range summaryDestinations(destinations) {
			if summary == nil || summary.path.display == stdioPath {
				continue
			}
			if pathsOverlap(summary.path, outputNamespace(*destinations.outputRoot, eventsOutputDirectory)) ||
				pathsOverlap(summary.path, outputNamespace(*destinations.outputRoot, reportsOutputDirectory)) {
				return fmt.Errorf(
					"summary output path %q conflicts with a reserved output namespace",
					summary.path.display,
				)
			}
		}
	}
	if inputRoot != nil {
		for _, summary := range summaryDestinations(destinations) {
			if summary != nil && summary.path.display != stdioPath && pathsOverlap(*inputRoot, summary.path) {
				return fmt.Errorf(
					"summary output path %q conflicts with the input event directory",
					summary.path.display,
				)
			}
		}
	}
	return nil
}

func summaryDestinations(destinations processingDestinations) []*outputDestination {
	summaries := make([]*outputDestination, 0, len(destinations.summaryFiles)+1)
	summaries = append(summaries, destinations.summaryFiles...)
	if destinations.summaryJSONFile != nil {
		summaries = append(summaries, destinations.summaryJSONFile)
	}
	return summaries
}

func validateOutputDirectory(config processConfig, outputRoot filesystemPath) error {
	info, err := os.Stat(outputRoot.absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect output directory %q: %w", outputRoot.display, fserror.QuotePaths(err))
	}
	if !info.IsDir() {
		return fmt.Errorf("output path %q is not a directory", outputRoot.display)
	}
	if config.eventsDir == "" || config.overwrite {
		return validateOutputNamespaces(config, outputRoot)
	}
	empty, err := directoryIsEmpty(outputRoot.absolute)
	if err != nil {
		return fmt.Errorf("read output directory %q: %w", outputRoot.display, fserror.QuotePaths(err))
	}
	if !empty {
		return fmt.Errorf(
			"output directory %q is not empty (use --overwrite to replace existing outputs)",
			outputRoot.display,
		)
	}
	return validateOutputNamespaces(config, outputRoot)
}

func validateEventsDirectory(inputRoot filesystemPath) error {
	info, err := os.Lstat(inputRoot.absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("events input directory %q does not exist", inputRoot.display)
	}
	if err != nil {
		return fmt.Errorf(
			"inspect events input directory %q: %w", inputRoot.display, fserror.QuotePaths(err),
		)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("events input path %q is a symbolic link", inputRoot.display)
	}
	if !info.IsDir() {
		return fmt.Errorf("events input path %q is not a directory", inputRoot.display)
	}
	return nil
}

func directoryIsEmpty(path string) (bool, error) {
	directory, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = directory.Close() }()

	_, err = directory.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	return false, err
}

func validateOutputNamespaces(config processConfig, outputRoot filesystemPath) error {
	for _, name := range activeOutputNamespaces(config) {
		root := filepath.Join(outputRoot.absolute, name)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && path == root {
				return fs.SkipAll
			}
			if err != nil {
				return fserror.QuotePaths(err)
			}
			if path == root && !entry.IsDir() {
				return fmt.Errorf("output namespace %q is not a directory", name)
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("output namespace %q contains symbolic link %q", name, path)
			}
			info, err := entry.Info()
			if err != nil {
				return fserror.QuotePaths(err)
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return fmt.Errorf("output namespace %q contains unsupported filesystem entry %q", name, path)
			}
			return nil
		})
		if err != nil {
			return fserror.QuotePaths(err)
		}
	}
	return nil
}

func activeOutputNamespaces(config processConfig) []string {
	names := make([]string, 0, 2)
	if config.mutatesEvent() {
		names = append(names, eventsOutputDirectory)
	}
	if config.generatesReport() {
		names = append(names, reportsOutputDirectory)
	}
	return names
}

// resolveDistinctDestination is a one-time preflight check: it compares every explicitly selected destination
// against every other destination reserved so far in this run and rejects a collision before any output is
// written. It intentionally stops there. It does not claim, lock, or otherwise track these paths for the rest of
// the run, and nothing later re-checks them against concurrent external modification: the CLI is a local tool that
// assumes a stable filesystem while it runs, not a service defending against other processes or users. If the
// filesystem changes after this check passes (or a case-folding or link-based alias this check cannot see causes an
// unresolved collision), the write itself fails with an ordinary error — see writeOutputFile and
// installTemporaryFileWithoutOverwrite in output.go — instead of being retried, reconciled, or specially handled.
// See "CLI Boundary" in docs/architecture.md and the FAQ entries "What happens if input or output directories
// change during processing?" and "Can two different input files produce the same output path?" in docs/FAQ.md.
func resolveDistinctDestination(
	display string,
	description string,
	reserved *[]reservedPath,
) (*outputDestination, error) {
	destination, err := newOutputDestination(display)
	if err != nil {
		return nil, fmt.Errorf("resolve %s path %q: %w", description, display, err)
	}
	if destination.path.display == stdioPath {
		return destination, nil
	}
	for _, previous := range *reserved {
		if sameFilesystemPath(destination.path, previous.path) {
			return nil, fmt.Errorf(
				"path %q is selected for both %s and %s",
				display,
				previous.description,
				description,
			)
		}
		if pathsOverlap(destination.path, previous.path) {
			return nil, fmt.Errorf(
				"path %q selected for %s overlaps %s path %q",
				display,
				description,
				previous.description,
				previous.path.display,
			)
		}
	}
	*reserved = append(*reserved, reservedPath{path: destination.path, description: description})
	return destination, nil
}

func outputNamespace(root filesystemPath, name string) filesystemPath {
	return filesystemPath{
		display:  filepath.Join(root.display, name),
		absolute: filepath.Join(root.absolute, name),
		resolved: filepath.Join(root.resolved, name),
	}
}

func eventOutputPath(config processConfig, input inputEvent) string {
	if config.eventPath != "" && config.eventOutput != "" {
		return config.eventOutput
	}
	return filepath.Join(config.outputDir, eventsOutputDirectory, eventOutputRelativePath(input))
}

func reportOutputPath(config processConfig, input inputEvent) string {
	if config.eventPath != "" && config.reportOutput != "" {
		return config.reportOutput
	}
	return filepath.Join(config.outputDir, reportsOutputDirectory, reportOutputRelativePath(input))
}

func eventOutputRelativePath(input inputEvent) string {
	if input.rel != "" {
		return safeOutputRelativePath(input.rel)
	}
	return safeOutputRelativePath(input.path)
}

// reportOutputRelativePath inserts a ".report" suffix before the file extension of the derived event
// path, making an auto-derived report name unlikely to collide with a real event's filename.
func reportOutputRelativePath(input inputEvent) string {
	relative := eventOutputRelativePath(input)
	dir, base := filepath.Split(relative)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(dir, stem+".report"+ext)
}

func safeOutputRelativePath(path string) string {
	cleanPath := filepath.Clean(path)
	if filepath.IsLocal(cleanPath) {
		return cleanPath
	}
	base := filepath.Base(cleanPath)
	if filepath.IsLocal(base) {
		return base
	}
	return stdinEventRelativePath
}
