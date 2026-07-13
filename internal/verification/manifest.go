package verification

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

var (
	requirementIDPattern = regexp.MustCompile(`^REQ-[0-9]{3}$`)
	testIDPattern        = regexp.MustCompile(`^TEST-[0-9]{3}$`)
)

type Manifest struct {
	Version      int      `yaml:"version"`
	Requirements []string `yaml:"requirements"`
	Groups       []string `yaml:"groups"`
	Tests        []Test   `yaml:"tests"`
}

type Test struct {
	ID             string       `yaml:"id"`
	Name           string       `yaml:"name"`
	Requirements   []string     `yaml:"requirements"`
	Groups         []string     `yaml:"groups"`
	Files          []string     `yaml:"files"`
	Command        []string     `yaml:"command"`
	Discovery      *GoDiscovery `yaml:"discovery,omitempty"`
	Evidence       string       `yaml:"evidence"`
	TimeoutSeconds int          `yaml:"timeout_seconds"`
}

type GoDiscovery struct {
	Packages []string `yaml:"packages"`
	Pattern  string   `yaml:"pattern"`
	Tags     []string `yaml:"tags,omitempty"`
}

func Load(path string) (Manifest, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	return Decode(payload)
}

func Decode(payload []byte) (Manifest, error) {
	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode verification manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New("decode verification manifest: multiple YAML documents are not allowed")
		}
		return Manifest{}, fmt.Errorf("decode verification manifest: %w", err)
	}
	return manifest, nil
}

func (m Manifest) Validate(root string) error {
	if m.Version != 1 {
		return fmt.Errorf("unsupported manifest version %d", m.Version)
	}
	if len(m.Requirements) == 0 {
		return errors.New("manifest has no requirements")
	}
	if len(m.Groups) == 0 {
		return errors.New("manifest has no groups")
	}
	if len(m.Tests) == 0 {
		return errors.New("manifest has no tests")
	}

	requirements, err := uniqueIDs("requirement", m.Requirements, requirementIDPattern)
	if err != nil {
		return err
	}
	groups, err := uniqueStrings("group", m.Groups)
	if err != nil {
		return err
	}

	testIDs := make(map[string]struct{}, len(m.Tests))
	commands := make(map[string]string, len(m.Tests))
	covered := make(map[string]struct{}, len(requirements))
	for index, test := range m.Tests {
		if !testIDPattern.MatchString(test.ID) {
			return fmt.Errorf("test %d has malformed id %q", index+1, test.ID)
		}
		if _, duplicate := testIDs[test.ID]; duplicate {
			return fmt.Errorf("duplicate test id %s", test.ID)
		}
		testIDs[test.ID] = struct{}{}
		if strings.TrimSpace(test.Name) == "" {
			return fmt.Errorf("%s has blank name", test.ID)
		}
		if strings.TrimSpace(test.Evidence) == "" {
			return fmt.Errorf("%s has blank evidence", test.ID)
		}
		if test.TimeoutSeconds <= 0 {
			return fmt.Errorf("%s has invalid timeout_seconds %d", test.ID, test.TimeoutSeconds)
		}
		if len(test.Requirements) == 0 {
			return fmt.Errorf("%s has no requirements", test.ID)
		}
		for _, requirement := range test.Requirements {
			if _, known := requirements[requirement]; !known {
				return fmt.Errorf("%s references unknown requirement %s", test.ID, requirement)
			}
			covered[requirement] = struct{}{}
		}
		if len(test.Groups) == 0 {
			return fmt.Errorf("%s has no groups", test.ID)
		}
		for _, group := range test.Groups {
			if _, known := groups[group]; !known {
				return fmt.Errorf("%s references unknown group %q", test.ID, group)
			}
		}
		if len(test.Files) == 0 {
			return fmt.Errorf("%s has no files", test.ID)
		}
		for _, path := range test.Files {
			if err := validatePath(root, path); err != nil {
				return fmt.Errorf("%s: %w", test.ID, err)
			}
		}
		if len(test.Command) == 0 {
			return fmt.Errorf("%s has no command", test.ID)
		}
		for _, arg := range test.Command {
			if arg == "" {
				return fmt.Errorf("%s command contains an empty argument", test.ID)
			}
		}
		commandKey := strings.Join(test.Command, "\x00")
		if owner, duplicate := commands[commandKey]; duplicate {
			return fmt.Errorf("duplicate test command owned by %s and %s", owner, test.ID)
		}
		commands[commandKey] = test.ID
		if recursivelyInvokesVerifyPlan(test.Command) {
			return fmt.Errorf("%s recursively invokes verify-plan", test.ID)
		}
		if commandHasRunFlag(test.Command) && test.Discovery == nil {
			return fmt.Errorf("%s focused command requires discovery metadata", test.ID)
		}
		if test.Discovery != nil {
			if len(test.Discovery.Packages) == 0 || strings.TrimSpace(test.Discovery.Pattern) == "" {
				return fmt.Errorf("%s has incomplete discovery metadata", test.ID)
			}
			if _, err := regexp.Compile(test.Discovery.Pattern); err != nil {
				return fmt.Errorf("%s has invalid discovery pattern: %w", test.ID, err)
			}
		}
	}

	for requirement := range requirements {
		if _, ok := covered[requirement]; !ok {
			return fmt.Errorf("requirement %s has no test coverage", requirement)
		}
	}
	return nil
}

func (m Manifest) Select(testID string, groups []string) ([]Test, error) {
	if testID != "" && len(groups) > 0 {
		return nil, errors.New("select either a test or groups, not both")
	}
	if testID != "" {
		for _, test := range m.Tests {
			if test.ID == testID {
				return []Test{test}, nil
			}
		}
		return nil, fmt.Errorf("unknown test id %s", testID)
	}
	if len(groups) == 0 {
		groups = []string{"plan"}
	}
	selectedGroups := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		selectedGroups[group] = struct{}{}
	}
	var selected []Test
	for _, test := range m.Tests {
		if intersects(test.Groups, selectedGroups) {
			selected = append(selected, test)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no tests selected for groups %s", strings.Join(groups, ","))
	}
	return selected, nil
}

func Run(ctx context.Context, root string, tests []Test, stdout, stderr io.Writer) error {
	for _, test := range tests {
		fmt.Fprintf(stdout, "==> %s %s\n", test.ID, test.Name)
		if test.Discovery != nil {
			count, err := DiscoverGoTests(ctx, root, test)
			if err != nil {
				return fmt.Errorf("%s discovery failed: %w", test.ID, err)
			}
			if count == 0 {
				return fmt.Errorf("%s discovery matched zero Go tests", test.ID)
			}
			fmt.Fprintf(stdout, "%s discovery matched %d Go tests\n", test.ID, count)
		}
		commandCtx, cancel := context.WithTimeout(ctx, time.Duration(test.TimeoutSeconds)*time.Second)
		command := exec.CommandContext(commandCtx, test.Command[0], test.Command[1:]...)
		command.Dir = root
		command.Stdout = stdout
		command.Stderr = stderr
		err := command.Run()
		cancel()
		if errors.Is(commandCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%s exceeded %d second runtime budget", test.ID, test.TimeoutSeconds)
		}
		if err != nil {
			return fmt.Errorf("%s command failed: %w", test.ID, err)
		}
	}
	return nil
}

func DiscoverGoTests(ctx context.Context, root string, test Test) (int, error) {
	if test.Discovery == nil {
		return 0, errors.New("test has no discovery metadata")
	}
	args := []string{"test"}
	if len(test.Discovery.Tags) > 0 {
		args = append(args, "-tags", strings.Join(test.Discovery.Tags, ","))
	}
	args = append(args, test.Discovery.Packages...)
	args = append(args, "-list", test.Discovery.Pattern)
	command := exec.CommandContext(ctx, "go", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	pattern, err := regexp.Compile(test.Discovery.Pattern)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, line := range strings.Split(string(output), "\n") {
		if pattern.MatchString(strings.TrimSpace(line)) {
			count++
		}
	}
	return count, nil
}

func uniqueIDs(kind string, values []string, pattern *regexp.Regexp) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !pattern.MatchString(value) {
			return nil, fmt.Errorf("malformed %s id %q", kind, value)
		}
		if _, duplicate := result[value]; duplicate {
			return nil, fmt.Errorf("duplicate %s id %s", kind, value)
		}
		result[value] = struct{}{}
	}
	return result, nil
}

func uniqueStrings(kind string, values []string) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("blank %s", kind)
		}
		if _, duplicate := result[value]; duplicate {
			return nil, fmt.Errorf("duplicate %s %q", kind, value)
		}
		result[value] = struct{}{}
	}
	return result, nil
}

func validatePath(root, path string) error {
	if filepath.IsAbs(path) || path == "" {
		return fmt.Errorf("invalid relative file path %q", path)
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("file path escapes repository: %q", path)
	}
	if _, err := os.Stat(filepath.Join(root, clean)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("missing file %q", path)
		}
		return err
	}
	return nil
}

func recursivelyInvokesVerifyPlan(command []string) bool {
	if filepath.Base(command[0]) != "make" {
		return false
	}
	for _, arg := range command[1:] {
		if arg == "verify-plan" || strings.HasPrefix(arg, "verify-plan=") {
			return true
		}
	}
	return false
}

func commandHasRunFlag(command []string) bool {
	for _, arg := range command {
		if arg == "-run" || strings.HasPrefix(arg, "-run=") {
			return true
		}
	}
	return false
}

func intersects(values []string, selected map[string]struct{}) bool {
	for _, value := range values {
		if _, ok := selected[value]; ok {
			return true
		}
	}
	return false
}

func SortTests(tests []Test) {
	sort.Slice(tests, func(i, j int) bool { return tests[i].ID < tests[j].ID })
}
