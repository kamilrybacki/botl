package runconfig

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestDefault(t *testing.T) {
	rc := Default()
	if rc.CloneMode != "shallow" {
		t.Errorf("CloneMode = %q, want %q", rc.CloneMode, "shallow")
	}
	if rc.Depth != 1 {
		t.Errorf("Depth = %d, want 1", rc.Depth)
	}
	if len(rc.BlockedPorts) != 0 {
		t.Errorf("BlockedPorts = %v, want empty", rc.BlockedPorts)
	}
	if rc.Timeout != 30*time.Minute {
		t.Errorf("Timeout = %v, want 30m", rc.Timeout)
	}
	if rc.Image != "botl:latest" {
		t.Errorf("Image = %q, want %q", rc.Image, "botl:latest")
	}
	if rc.OutputDir != "./botl-output" {
		t.Errorf("OutputDir = %q, want %q", rc.OutputDir, "./botl-output")
	}
	if rc.EnvVarKeys != nil {
		t.Errorf("EnvVarKeys = %v, want nil", rc.EnvVarKeys)
	}
}

func TestRunConfig_YAMLRoundTrip(t *testing.T) {
	original := RunConfig{
		CloneMode:    "deep",
		Depth:        0,
		BlockedPorts: []int{8080, 3000},
		Timeout:      45 * time.Minute,
		Image:        "botl:custom",
		OutputDir:    "/tmp/out",
		EnvVarKeys:   []string{"FOO", "BAR"},
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var loaded RunConfig
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if loaded.CloneMode != original.CloneMode {
		t.Errorf("CloneMode = %q, want %q", loaded.CloneMode, original.CloneMode)
	}
	if loaded.Depth != original.Depth {
		t.Errorf("Depth = %d, want %d", loaded.Depth, original.Depth)
	}
	if len(loaded.BlockedPorts) != 2 || loaded.BlockedPorts[0] != 8080 {
		t.Errorf("BlockedPorts = %v, want %v", loaded.BlockedPorts, original.BlockedPorts)
	}
	if loaded.Timeout != original.Timeout {
		t.Errorf("Timeout = %v, want %v", loaded.Timeout, original.Timeout)
	}
	if loaded.Image != original.Image {
		t.Errorf("Image = %q, want %q", loaded.Image, original.Image)
	}
	if loaded.OutputDir != original.OutputDir {
		t.Errorf("OutputDir = %q, want %q", loaded.OutputDir, original.OutputDir)
	}
	if len(loaded.EnvVarKeys) != 2 || loaded.EnvVarKeys[0] != "FOO" {
		t.Errorf("EnvVarKeys = %v, want %v", loaded.EnvVarKeys, original.EnvVarKeys)
	}
}
