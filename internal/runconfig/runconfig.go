package runconfig

import "time"

// RunConfig holds all run-level configuration that is captured in session
// records and reusable profiles. Mounts are intentionally excluded because
// host-specific paths are not portable across machines.
type RunConfig struct {
	CloneMode    string        `yaml:"clone_mode"`
	Depth        int           `yaml:"depth"`
	BlockedPorts []int         `yaml:"blocked_ports"`
	Timeout      time.Duration `yaml:"timeout"`
	Image        string        `yaml:"image"`
	OutputDir    string        `yaml:"output_dir"`
	EnvVarKeys   []string      `yaml:"env_var_keys,omitempty"`
}

// Default returns a RunConfig with built-in defaults matching botl's
// zero-configuration behavior.
func Default() RunConfig {
	return RunConfig{
		CloneMode:    "shallow",
		Depth:        1,
		BlockedPorts: []int{},
		Timeout:      30 * time.Minute,
		Image:        "botl:latest",
		OutputDir:    "./botl-output",
	}
}
