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
)

const (
	eventsOutputDirectory  = "events"
	reportsOutputDirectory = "reports"
)

type inputEvent struct {
	path string
	rel  string
}

// filesystemPath retains both the absolute spelling of a path and the form obtained by
// resolving symbolic links in its longest existing prefix.
type filesystemPath struct {
	display  string
	absolute string
	resolved string
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
		return filesystemPath{}, fmt.Errorf("resolve absolute path for %q: %w", display, err)
	}
	absolute = filepath.Clean(absolute)
	resolved, _, err := resolveExistingPathPrefix(absolute)
	if err != nil {
		return filesystemPath{}, fmt.Errorf("resolve symlinks for %q: %w", display, err)
	}
	return filesystemPath{display: display, absolute: absolute, resolved: resolved}, nil
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
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func pathsOverlap(left, right filesystemPath) bool {
	return pathContains(left, right) || pathContains(right, left)
}

func buildProcessingDestinations(config processConfig) (processingDestinations, error) {
	destinations := processingDestinations{}
	var inputRoot *filesystemPath
	if config.outputDir != "" {
		outputRoot, err := newFilesystemPath(config.outputDir)
		if err != nil {
			return processingDestinations{}, fmt.Errorf("resolve output directory: %w", err)
		}
		destinations.outputRoot = &outputRoot
		if err := validateOutputDirectory(config, outputRoot); err != nil {
			return processingDestinations{}, err
		}
	}

	if config.eventsDir != "" && destinations.outputRoot != nil {
		root, err := newFilesystemPath(config.eventsDir)
		if err != nil {
			return processingDestinations{}, fmt.Errorf("resolve events input directory: %w", err)
		}
		inputRoot = &root
		if pathsOverlap(root, *destinations.outputRoot) {
			return processingDestinations{}, errors.New("input and output directory trees must not overlap")
		}
	}

	reserved := make(map[string]string)
	if config.eventPath != "" && config.eventPath != stdioPath {
		inputPath, err := newFilesystemPath(config.eventPath)
		if err != nil {
			return processingDestinations{}, fmt.Errorf("resolve input event %q: %w", config.eventPath, err)
		}
		reserved[inputPath.resolved] = fmt.Sprintf("input event %q", config.eventPath)
	}

	var err error
	if config.eventPath != "" {
		input := inputEvent{path: config.eventPath, rel: stdinEventRelativePath}
		if config.eventPath != stdioPath {
			input.rel = ""
		}
		if config.mutatesEvent() {
			destinations.eventOutput, err = resolveDistinctDestination(
				eventOutputPath(config, input), "processed event", reserved,
			)
			if err != nil {
				return processingDestinations{}, err
			}
		}
		if config.generatesReport() {
			destinations.reportOutput, err = resolveDistinctDestination(
				reportOutputPath(config, input), "processing report", reserved,
			)
			if err != nil {
				return processingDestinations{}, err
			}
		}
	}
	if config.summaryFile != "" {
		summaryFile, resolveErr := resolveDistinctDestination(
			config.summaryFile, "human-readable summary", reserved,
		)
		if resolveErr != nil {
			return processingDestinations{}, resolveErr
		}
		destinations.summaryFiles = append(destinations.summaryFiles, summaryFile)
	}
	if config.summaryJSONFile != "" {
		destinations.summaryJSONFile, err = resolveDistinctDestination(
			config.summaryJSONFile, "JSON summary", reserved,
		)
		if err != nil {
			return processingDestinations{}, err
		}
	}

	if config.eventsDir != "" && !config.quiet && config.summaryFile != stdioPath {
		destinations.summaryFiles = append([]*outputDestination{displayDestination(stdioPath)}, destinations.summaryFiles...)
	}

	if destinations.outputRoot != nil {
		for _, summary := range summaryDestinations(destinations) {
			if summary == nil || summary.path.display == stdioPath {
				continue
			}
			if summary.path.resolved == destinations.outputRoot.resolved ||
				pathContains(outputNamespace(*destinations.outputRoot, eventsOutputDirectory), summary.path) ||
				pathContains(outputNamespace(*destinations.outputRoot, reportsOutputDirectory), summary.path) {
				return processingDestinations{}, fmt.Errorf(
					"summary output path %q conflicts with a reserved output namespace",
					summary.path.display,
				)
			}
		}
	}
	if inputRoot != nil {
		for _, summary := range summaryDestinations(destinations) {
			if summary != nil && summary.path.display != stdioPath && pathContains(*inputRoot, summary.path) {
				return processingDestinations{}, fmt.Errorf(
					"summary output path %q conflicts with the input event directory",
					summary.path.display,
				)
			}
		}
	}
	return destinations, nil
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
		return fmt.Errorf("inspect output directory %q: %w", outputRoot.display, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("output path %q is not a directory", outputRoot.display)
	}
	if config.eventsDir == "" || config.overwrite {
		return validateOutputNamespaces(config, outputRoot)
	}
	empty, err := directoryIsEmpty(outputRoot.absolute)
	if err != nil {
		return fmt.Errorf("read output directory %q: %w", outputRoot.display, err)
	}
	if !empty {
		return fmt.Errorf("output directory %q is not empty (use --overwrite to replace existing outputs)", outputRoot.display)
	}
	return validateOutputNamespaces(config, outputRoot)
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
	names := make([]string, 0, 2)
	if config.mutatesEvent() {
		names = append(names, eventsOutputDirectory)
	}
	if config.generatesReport() {
		names = append(names, reportsOutputDirectory)
	}
	for _, name := range names {
		root := filepath.Join(outputRoot.absolute, name)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && path == root {
				return fs.SkipAll
			}
			if err != nil {
				return err
			}
			if path == root && !entry.IsDir() {
				return fmt.Errorf("output namespace %q is not a directory", name)
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("output namespace %q contains symbolic link %q", name, path)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func resolveDistinctDestination(
	display string,
	description string,
	reserved map[string]string,
) (*outputDestination, error) {
	destination, err := newOutputDestination(display)
	if err != nil {
		return nil, fmt.Errorf("resolve %s path %q: %w", description, display, err)
	}
	if destination.path.display == stdioPath {
		return destination, nil
	}
	if previous, collision := reserved[destination.path.resolved]; collision {
		return nil, fmt.Errorf("path %q is selected for both %s and %s", display, previous, description)
	}
	reserved[destination.path.resolved] = description
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
	return filepath.Join(config.outputDir, reportsOutputDirectory, eventOutputRelativePath(input))
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
