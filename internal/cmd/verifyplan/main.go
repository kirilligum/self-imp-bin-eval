package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kirilligum/self-imp-bin-eval/internal/verification"
)

func main() {
	manifestPath := flag.String("manifest", "docs/test-matrix.yml", "verification manifest path")
	testID := flag.String("test", "", "single TEST id")
	groupList := flag.String("groups", "", "comma-separated verification groups")
	checkOnly := flag.Bool("check", false, "validate without executing tests")
	flag.Parse()

	root, err := repositoryRoot()
	if err != nil {
		fatal(err)
	}
	path := *manifestPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	manifest, err := verification.Load(path)
	if err != nil {
		fatal(err)
	}
	if err := manifest.Validate(root); err != nil {
		fatal(err)
	}
	if *checkOnly {
		fmt.Println("verification manifest ok")
		return
	}
	groups := splitGroups(*groupList)
	tests, err := manifest.Select(*testID, groups)
	if err != nil {
		fatal(err)
	}
	verification.SortTests(tests)
	if err := verification.Run(context.Background(), root, tests, os.Stdout, os.Stderr); err != nil {
		fatal(err)
	}
}

func repositoryRoot() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for current := workingDirectory; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod not found from %s", workingDirectory)
		}
	}
}

func splitGroups(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	groups := make([]string, 0, len(parts))
	for _, part := range parts {
		if group := strings.TrimSpace(part); group != "" {
			groups = append(groups, group)
		}
	}
	return groups
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
