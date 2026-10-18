package verification

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Rendering both native models guards against a fixture reaching production
// persistence or joining the retained workers' control network.
func TestTemporalFixtureAndSharedPersistenceAreIsolated(t *testing.T) {
	root := repositoryRoot(t)
	for _, production := range []bool{false, true} {
		name := "fixture"
		if production {
			name = "production"
		}
		t.Run(name, func(t *testing.T) {
			args := []string{"compose", "--env-file", "deploy/compose/.env.example", "-f", "deploy/compose/docker-compose.yml"}
			if production {
				args = append(args, "-f", "deploy/compose/compose.temporal-shared.yml")
			} else {
				args = append(args, "-f", "deploy/test/compose.app.yml")
			}
			if !production {
				args = append(args, "--profile", "test", "--profile", "app", "--profile", "deterministic")
			}
			args = append(args, "config", "--format", "json")
			cmd := exec.Command("docker", args...)
			cmd.Dir = root
			// Keep configuration independent of operator credentials/environment.
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
			if production {
				cmd.Env = append(cmd.Env, "TEMPORAL_POSTGRES_HOST=postgres-shared", "TEMPORAL_POSTGRES_USER=temporal", "TEMPORAL_POSTGRES_PASSWORD=fixture-only", "TEMPORAL_SKIP_DB_CREATE=true", "TEMPORAL_SKIP_SCHEMA_SETUP=true")
			}
			output, err := cmd.Output()
			require.NoError(t, err, "native Compose configuration must render")
			var model struct {
				Services map[string]struct {
					Environment map[string]string `json:"environment"`
					Networks    map[string]any    `json:"networks"`
					DependsOn   map[string]any    `json:"depends_on"`
				} `json:"services"`
			}
			require.NoError(t, json.Unmarshal(output, &model))
			temporal := model.Services["temporal"]
			require.Equal(t, "postgres12", temporal.Environment["DB"])
			require.Equal(t, "temporal", temporal.Environment["DBNAME"])
			require.Equal(t, "temporal_visibility", temporal.Environment["VISIBILITY_DBNAME"])
			require.NotContains(t, temporal.DependsOn, "postgres")
			require.Contains(t, temporal.Networks, "default")
			if production {
				require.NotContains(t, model.Services, "postgres")
				require.Equal(t, "postgres-shared", temporal.Environment["POSTGRES_SEEDS"])
				require.Equal(t, "temporal", temporal.Environment["POSTGRES_USER"])
				require.Equal(t, "true", temporal.Environment["SKIP_DB_CREATE"])
				require.Equal(t, "true", temporal.Environment["SKIP_SCHEMA_SETUP"])
				require.Contains(t, temporal.Networks, "agent-platform-control")
			} else {
				require.Contains(t, model.Services, "postgres")
				require.Equal(t, "postgres", temporal.Environment["POSTGRES_SEEDS"])
				require.Equal(t, "bin_eval", temporal.Environment["POSTGRES_USER"])
				require.Equal(t, "false", temporal.Environment["SKIP_DB_CREATE"])
				require.Equal(t, "false", temporal.Environment["SKIP_SCHEMA_SETUP"])
				require.NotContains(t, temporal.Networks, "agent-platform-control")
			}
		})
	}
	require.FileExists(t, filepath.Join(root, "docs", "temporal-postgres.md"))
}
