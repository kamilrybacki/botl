package container

import (
	"strings"
	"testing"
)

func TestEmbeddedDockerContext(t *testing.T) {
	entries, err := dockerCtx.ReadDir("dockerctx")
	if err != nil {
		t.Fatalf("failed to read embedded dockerctx: %v", err)
	}

	expectedFiles := map[string]bool{
		"Dockerfile":    false,
		"entrypoint.sh": false,
		"botl-postrun":  false,
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
		if !strings.Contains(content, c.substring) {
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
		if !strings.Contains(content, c.substring) {
			t.Errorf("entrypoint.sh missing %s (expected substring %q)", c.desc, c.substring)
		}
	}
}

func TestEmbeddedPostrunBinary(t *testing.T) {
	data, err := dockerCtx.ReadFile("dockerctx/botl-postrun")
	if err != nil {
		t.Fatalf("botl-postrun not found in embedded dockerctx: %v", err)
	}
	if len(data) == 0 {
		t.Error("embedded botl-postrun binary is empty")
	}
	// ELF magic: \x7fELF
	if len(data) < 4 || data[0] != 0x7f || data[1] != 'E' || data[2] != 'L' || data[3] != 'F' {
		t.Error("embedded botl-postrun does not look like an ELF binary")
	}
}

