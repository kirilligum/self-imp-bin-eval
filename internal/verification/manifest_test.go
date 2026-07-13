package verification

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestP06CIContract(t *testing.T) {
	root := repositoryRoot(t)
	workflowPayload, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	require.NoError(t, err)
	var workflow struct {
		Jobs map[string]struct {
			If    string `yaml:"if"`
			Steps []struct {
				Uses            string            `yaml:"uses"`
				Run             string            `yaml:"run"`
				Env             map[string]string `yaml:"env"`
				ContinueOnError bool              `yaml:"continue-on-error"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(workflowPayload, &workflow))
	deterministic, ok := workflow.Jobs["deterministic"]
	require.True(t, ok, "missing deterministic CI job")
	live, ok := workflow.Jobs["live"]
	require.True(t, ok, "missing live CI job")
	require.Contains(t, live.If, "refs/heads/master")
	require.Contains(t, live.If, "release")
	require.Contains(t, string(workflowPayload), "runs-on: [self-hosted, linux, x64, bin-eval-live]")
	require.Contains(t, string(workflowPayload), "docker network connect --alias bin-eval-litellm")
	require.Contains(t, string(workflowPayload), "Stop live app containers")
	runnerInstaller, err := os.ReadFile(filepath.Join(root, "scripts", "install-live-ci-runner.sh"))
	require.NoError(t, err)
	require.Contains(t, string(runnerInstaller), "ExecStart=/usr/bin/sg docker")
	require.Contains(t, string(runnerInstaller), "KillMode=control-group")

	assertCIJob := func(t *testing.T, steps []struct {
		Uses            string            `yaml:"uses"`
		Run             string            `yaml:"run"`
		Env             map[string]string `yaml:"env"`
		ContinueOnError bool              `yaml:"continue-on-error"`
	}, endpointFragment string) string {
		t.Helper()
		var allRuns strings.Builder
		var endpointFound bool
		for _, step := range steps {
			require.False(t, step.ContinueOnError)
			allRuns.WriteString(step.Run)
			allRuns.WriteByte('\n')
			if strings.Contains(step.Env["BIN_EVAL_LLM_BASE_URL"], endpointFragment) {
				endpointFound = true
			}
		}
		require.True(t, endpointFound, "job does not configure expected LLM endpoint class")
		require.Contains(t, allRuns.String(), "scripts/run_e2e.sh")
		require.Contains(t, allRuns.String(), "BIN_EVAL_EXTERNAL_STACK=true")
		return allRuns.String()
	}
	deterministicRuns := assertCIJob(t, deterministic.Steps, "llm-fixture")
	liveRuns := assertCIJob(t, live.Steps, "secrets.")
	require.Contains(t, deterministicRuns, "--profile deterministic")
	require.Contains(t, deterministicRuns, "--profile app")
	require.Contains(t, liveRuns, "--profile app")
	require.NotContains(t, liveRuns, "--profile deterministic")

	composePayload, err := os.ReadFile(filepath.Join(root, "deploy", "compose", "docker-compose.yml"))
	require.NoError(t, err)
	var compose struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal(composePayload, &compose))
	for _, service := range []string{"postgres", "temporal", "garage", "api", "worker", "llm-fixture"} {
		require.Contains(t, compose.Services, service)
	}
	require.Equal(t, compose.Services["api"]["build"], compose.Services["worker"]["build"])
	dockerfilePayload, err := os.ReadFile(filepath.Join(root, "deploy", "compose", "Dockerfile"))
	require.NoError(t, err)
	require.Contains(t, string(dockerfilePayload), "COPY --from=build /src/migrations /migrations")
	for _, path := range []string{"scripts/validate_local_runtime_contract.sh", "scripts/validate_docs_curl.sh"} {
		payload, readErr := os.ReadFile(filepath.Join(root, path))
		require.NoError(t, readErr)
		require.NotContains(t, string(payload), "rg ", "%s depends on undeclared ripgrep tooling", path)
	}

	for _, path := range []string{"internal", "cmd/bin-eval-api", "cmd/bin-eval-worker"} {
		err := filepath.WalkDir(filepath.Join(root, path), func(filePath string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || filepath.Ext(filePath) != ".go" || strings.HasSuffix(filePath, "_test.go") {
				return err
			}
			payload, readErr := os.ReadFile(filePath)
			require.NoError(t, readErr)
			require.NotContains(t, strings.ToLower(string(payload)), "cache hit")
			return nil
		})
		require.NoError(t, err)
	}
}

func TestManifestValidationAcceptsValidManifest(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "test.go"), []byte("package example\n"), 0o600))

	manifest := validManifest()
	require.NoError(t, manifest.Validate(root))
}

func TestManifestValidationRejectsInvalidContracts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
		want   string
	}{
		{
			name: "duplicate requirement",
			mutate: func(manifest *Manifest) {
				manifest.Requirements = append(manifest.Requirements, manifest.Requirements[0])
			},
			want: "duplicate requirement id",
		},
		{
			name: "duplicate test id",
			mutate: func(manifest *Manifest) {
				duplicate := manifest.Tests[0]
				duplicate.Command = []string{"go", "test", "./other"}
				manifest.Tests = append(manifest.Tests, duplicate)
			},
			want: "duplicate test id",
		},
		{
			name: "duplicate command",
			mutate: func(manifest *Manifest) {
				duplicate := manifest.Tests[0]
				duplicate.ID = "TEST-102"
				manifest.Tests = append(manifest.Tests, duplicate)
			},
			want: "duplicate test command",
		},
		{
			name: "missing requirement coverage",
			mutate: func(manifest *Manifest) {
				manifest.Requirements = append(manifest.Requirements, "REQ-002")
			},
			want: "has no test coverage",
		},
		{
			name: "invalid runtime budget",
			mutate: func(manifest *Manifest) {
				manifest.Tests[0].TimeoutSeconds = 0
			},
			want: "invalid timeout_seconds",
		},
		{
			name: "unknown group",
			mutate: func(manifest *Manifest) {
				manifest.Tests[0].Groups = []string{"unknown"}
			},
			want: "unknown group",
		},
		{
			name: "missing file",
			mutate: func(manifest *Manifest) {
				manifest.Tests[0].Files = []string{"missing.go"}
			},
			want: "missing file",
		},
		{
			name: "recursive command",
			mutate: func(manifest *Manifest) {
				manifest.Tests[0].Command = []string{"make", "verify-plan"}
			},
			want: "recursively invokes verify-plan",
		},
		{
			name: "focused command without discovery",
			mutate: func(manifest *Manifest) {
				manifest.Tests[0].Discovery = nil
			},
			want: "requires discovery metadata",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "test.go"), []byte("package example\n"), 0o600))
			manifest := validManifest()
			tt.mutate(&manifest)
			require.ErrorContains(t, manifest.Validate(root), tt.want)
		})
	}
}

func TestManifestDecodeRejectsMalformedAndUnknownFields(t *testing.T) {
	_, err := Decode([]byte("version: [\n"))
	require.Error(t, err)

	_, err = Decode([]byte("version: 1\nrequirements: []\ngroups: []\ntests: []\nunknown: true\n"))
	require.ErrorContains(t, err, "field unknown not found")
}

func TestManifestDiscoveryRejectsZeroMatchingGoTests(t *testing.T) {
	test := Test{
		ID: "TEST-101",
		Discovery: &GoDiscovery{
			Packages: []string{"./internal/verification"},
			Pattern:  "^TestDoesNotExist$",
		},
	}

	count, err := DiscoverGoTests(context.Background(), repositoryRoot(t), test)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestManifestDiscoveryFindsFocusedGoTest(t *testing.T) {
	test := Test{
		ID: "TEST-101",
		Discovery: &GoDiscovery{
			Packages: []string{"./internal/verification"},
			Pattern:  "^TestManifestValidationAcceptsValidManifest$",
		},
	}

	count, err := DiscoverGoTests(context.Background(), repositoryRoot(t), test)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func validManifest() Manifest {
	return Manifest{
		Version:      1,
		Requirements: []string{"REQ-001"},
		Groups:       []string{"unit"},
		Tests: []Test{{
			ID:           "TEST-101",
			Name:         "manifest",
			Requirements: []string{"REQ-001"},
			Groups:       []string{"unit"},
			Files:        []string{"test.go"},
			Command:      []string{"go", "test", "./internal/verification", "-run", "^TestManifest", "-count=1"},
			Discovery: &GoDiscovery{
				Packages: []string{"./internal/verification"},
				Pattern:  "^TestManifest",
			},
			Evidence:       "test output",
			TimeoutSeconds: 120,
		}},
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return root
}
