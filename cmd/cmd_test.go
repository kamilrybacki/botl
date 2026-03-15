package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCommand(t *testing.T) {
	rootCmd.SetArgs([]string{})
	// Root command should succeed (prints help)
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("root command failed: %v", err)
	}
}

func TestRunCommand_MissingArgs(t *testing.T) {
	rootCmd.SetArgs([]string{"run"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("run without args should fail")
	}
}

func TestRunCommand_MissingClaudeDir(t *testing.T) {
	// Set HOME to a temp dir without .claude
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	rootCmd.SetArgs([]string{"run", "https://github.com/user/repo"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("run should fail when ~/.claude is missing")
	}
}

func TestRunCommand_ClaudeDirExists(t *testing.T) {
	// Create a temp home with .claude directory
	tmpHome := t.TempDir()
	os.MkdirAll(filepath.Join(tmpHome, ".claude"), 0755)
	t.Setenv("HOME", tmpHome)

	// This will fail at the Docker check (no docker), but it should get past
	// the ~/.claude validation. We check the error message.
	rootCmd.SetArgs([]string{"run", "https://github.com/user/repo"})
	err := rootCmd.Execute()
	if err == nil {
		t.Skip("docker is available, skipping")
	}
	// Should NOT be the "~/.claude not found" error
	errMsg := err.Error()
	if strings.Contains(errMsg, ".claude not found") {
		t.Errorf("unexpected error about .claude: %v", err)
	}
}

func TestBuildCommand_Flags(t *testing.T) {
	cmd := buildCmd

	imageFlag := cmd.Flags().Lookup("image")
	if imageFlag == nil {
		t.Fatal("build command missing --image flag")
	}
	if imageFlag.DefValue != "botl:latest" {
		t.Errorf("--image default = %q, want %q", imageFlag.DefValue, "botl:latest")
	}
}

func TestRunCommand_Flags(t *testing.T) {
	cmd := runCmd

	flagTests := []struct {
		name     string
		defValue string
	}{
		{"branch", ""},
		{"depth", "1"},
		{"prompt", ""},
		{"mount-packages", "true"},
		{"timeout", "30m0s"},
		{"image", "botl:latest"},
		{"output-dir", "./botl-output"},
	}

	for _, tt := range flagTests {
		f := cmd.Flags().Lookup(tt.name)
		if f == nil {
			t.Errorf("run command missing --%s flag", tt.name)
			continue
		}
		if f.DefValue != tt.defValue {
			t.Errorf("--%s default = %q, want %q", tt.name, f.DefValue, tt.defValue)
		}
	}
}

func TestRunCommand_NoAPIKeyFlag(t *testing.T) {
	cmd := runCmd
	if cmd.Flags().Lookup("api-key") != nil {
		t.Error("run command should not have --api-key flag (removed in favor of OAuth)")
	}
}

func TestRunCommand_NewFlags(t *testing.T) {
	cmd := runCmd
	flagTests := []struct {
		name string
	}{
		{"clone-mode"},
		{"blocked-ports"},
	}
	for _, tt := range flagTests {
		f := cmd.Flags().Lookup(tt.name)
		if f == nil {
			t.Errorf("run command missing --%s flag", tt.name)
		}
	}
}

func TestParsePorts(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int
		wantErr bool
	}{
		{"empty", "", []int{}, false},
		{"single", "8080", []int{8080}, false},
		{"multiple", "80, 443, 8080", []int{80, 443, 8080}, false},
		{"trailing comma", "80,", []int{80}, false},
		{"spaces", "  80 , 443 ", []int{80, 443}, false},
		{"non-numeric", "abc", nil, true},
		{"zero", "0", nil, true},
		{"too high", "65536", nil, true},
		{"mixed valid invalid", "80, 99999", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePorts(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parsePorts(%q) should error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePorts(%q) unexpected error: %v", tt.input, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parsePorts(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parsePorts(%q)[%d] = %d, want %d", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

