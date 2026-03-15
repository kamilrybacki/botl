package container

import (
	"testing"
)

func TestParseMount_Valid(t *testing.T) {
	tests := []struct {
		input  string
		source string
		target string
	}{
		{"/host/path:/container/path", "/host/path", "/container/path"},
		{"/a:/b", "/a", "/b"},
		{"./relative:/abs", "./relative", "/abs"},
		{"/path/with spaces:/other/path", "/path/with spaces", "/other/path"},
	}

	for _, tt := range tests {
		m, err := ParseMount(tt.input)
		if err != nil {
			t.Errorf("ParseMount(%q) returned error: %v", tt.input, err)
			continue
		}
		if m.Source != tt.source {
			t.Errorf("ParseMount(%q).Source = %q, want %q", tt.input, m.Source, tt.source)
		}
		if m.Target != tt.target {
			t.Errorf("ParseMount(%q).Target = %q, want %q", tt.input, m.Target, tt.target)
		}
	}
}

func TestParseMount_Invalid(t *testing.T) {
	tests := []string{
		"",
		"nodelimiter",
		":/missing-source",
		"/missing-target:",
		":",
	}

	for _, input := range tests {
		_, err := ParseMount(input)
		if err == nil {
			t.Errorf("ParseMount(%q) expected error, got nil", input)
		}
	}
}

func TestParseMount_ColonInPath(t *testing.T) {
	// SplitN with 2 means only first colon is used as delimiter
	m, err := ParseMount("/host:/container:extra")
	if err != nil {
		t.Fatalf("ParseMount with extra colon returned error: %v", err)
	}
	if m.Source != "/host" {
		t.Errorf("Source = %q, want /host", m.Source)
	}
	if m.Target != "/container:extra" {
		t.Errorf("Target = %q, want /container:extra", m.Target)
	}
}
