package container

import (
	"testing"
)

func TestEmbeddedDockerContext(t *testing.T) {
	entries, err := dockerCtx.ReadDir("dockerctx")
	if err != nil {
		t.Fatalf("failed to read embedded dockerctx: %v", err)
	}

	expectedFiles := map[string]bool{
		"Dockerfile":   false,
		"entrypoint.sh": false,
	}

	for _, entry := range entries {
		if _, ok := expectedFiles[entry.Name()]; ok {
			expectedFiles[entry.Name()] = true
		}
	}

	for name, found := range expectedFiles {
		if !found {
			t.Errorf("expected embedded file %q not found in dockerctx", name)
		}
	}
}

func TestEmbeddedDockerfileContent(t *testing.T) {
	data, err := dockerCtx.ReadFile("dockerctx/Dockerfile")
	if err != nil {
		t.Fatalf("failed to read embedded Dockerfile: %v", err)
	}

	content := string(data)

	checks := []struct {
		substring string
		desc      string
	}{
		{"FROM node:22-slim", "base image"},
		{"git", "git installation"},
		{"claude-code", "claude-code installation"},
		{"ENTRYPOINT", "entrypoint directive"},
		{"botl-postrun", "postrun binary copy"},
	}

	for _, c := range checks {
		if !containsStr(content, c.substring) {
			t.Errorf("Dockerfile missing %s (expected substring %q)", c.desc, c.substring)
		}
	}
}

func TestEmbeddedEntrypointContent(t *testing.T) {
	data, err := dockerCtx.ReadFile("dockerctx/entrypoint.sh")
	if err != nil {
		t.Fatalf("failed to read embedded entrypoint.sh: %v", err)
	}

	content := string(data)

	checks := []struct {
		substring string
		desc      string
	}{
		{"BOTL_REPO_URL", "repo URL env var"},
		{"git clone", "git clone command"},
		{"claude --dangerously-skip-permissions", "claude invocation"},
		{"botl-postrun", "postrun invocation"},
		{"BOTL_INITIAL_HEAD", "initial head recording"},
	}

	for _, c := range checks {
		if !containsStr(content, c.substring) {
			t.Errorf("entrypoint.sh missing %s (expected substring %q)", c.desc, c.substring)
		}
	}
}

func TestFindModuleRoot(t *testing.T) {
	root, err := findModuleRoot()
	if err != nil {
		t.Fatalf("findModuleRoot() returned error: %v", err)
	}
	if root == "" {
		t.Error("findModuleRoot() returned empty string")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
