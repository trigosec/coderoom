// Command codex-upgrade-compare compares two transcript version directories and
// validates that the newer recordings preserve the broad behavioral signals
// present in the older version.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/trigosec/coderoom/internal/transcript"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("usage: codex-upgrade-compare <previous-version-dir> <current-version-dir>")
	}
	return compareVersionDirs(filepath.Clean(os.Args[1]), filepath.Clean(os.Args[2]))
}

func compareVersionDirs(previousDir, currentDir string) error {
	previousCases, err := listCaseDirs(previousDir)
	if err != nil {
		return err
	}
	currentCases, err := listCaseDirs(currentDir)
	if err != nil {
		return err
	}
	if !slices.Equal(previousCases, currentCases) {
		return fmt.Errorf("case directories differ: previous=%v current=%v", previousCases, currentCases)
	}
	for _, name := range previousCases {
		previousOutput, err := readOutput(filepath.Join(previousDir, name, "output.transcript"))
		if err != nil {
			return fmt.Errorf("%s previous transcript: %w", name, err)
		}
		currentOutput, err := readOutput(filepath.Join(currentDir, name, "output.transcript"))
		if err != nil {
			return fmt.Errorf("%s current transcript: %w", name, err)
		}
		if err := transcript.CompareUpgradeOutputs(previousOutput, currentOutput); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func listCaseDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read version dir %q: %w", root, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	slices.Sort(names)
	return names, nil
}

func readOutput(path string) (transcript.Output, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return transcript.Output{}, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	output, _, err := transcript.ReadOutput(file)
	if err != nil {
		return transcript.Output{}, fmt.Errorf("read %q: %w", path, err)
	}
	return output, nil
}
