package artifacts

import "testing"

func TestSafeArtifactPath(t *testing.T) {
	got, err := safeArtifactPath("checklists/id/llm/prompt/attempt-1/request.json")
	if err != nil {
		t.Fatalf("safeArtifactPath() error = %v", err)
	}
	if got != "checklists/id/llm/prompt/attempt-1/request.json" {
		t.Fatalf("safeArtifactPath() = %q", got)
	}
	for _, key := range []string{"", "/absolute", "../escape", "path/../../escape", `path\escape`} {
		if _, err := safeArtifactPath(key); err == nil {
			t.Fatalf("safeArtifactPath(%q) succeeded", key)
		}
	}
}
