package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kamilrybacki/botl/internal/ansi"
	"github.com/kamilrybacki/botl/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure botl defaults via interactive TUI",
	RunE:  runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	cfgPath := config.Path()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "botl: warning: could not load config: %v\n", err)
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("cannot enter raw terminal mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState) //nolint:errcheck

	fmt.Print(ansi.CursorHide)
	defer fmt.Print(ansi.CursorShow)

	selected := 0
	editing := false
	editBuf := ""
	editErr := ""
	items := 2

	renderMenu := func() {
		cloneLabel := "shallow (sanitized, depth=1)"
		if cfg.Clone.Mode == "deep" {
			cloneLabel = "deep (full history)"
		}

		portsLabel := "none"
		if len(cfg.Network.BlockedPorts) > 0 {
			parts := make([]string, len(cfg.Network.BlockedPorts))
			for i, p := range cfg.Network.BlockedPorts {
				parts[i] = strconv.Itoa(p)
			}
			portsLabel = strings.Join(parts, ", ")
		}

		fmt.Print("\r" + ansi.ClearLine)
		fmt.Printf("  %sbotl configuration%s (%s)\r\n", ansi.Bold, ansi.Reset, cfgPath)
		fmt.Print("\r" + ansi.ClearLine)
		fmt.Printf("  %s─────────────────────────────────────────────────%s\r\n", ansi.Dim, ansi.Reset)
		fmt.Print("\r" + ansi.ClearLine + "\r\n")

		fmt.Print("\r" + ansi.ClearLine)
		if selected == 0 {
			fmt.Printf("  %s▸ Clone mode        %s%s%s\r\n", ansi.Green, ansi.Cyan, cloneLabel, ansi.Reset)
		} else {
			fmt.Printf("    %sClone mode        %s%s\r\n", ansi.Dim, cloneLabel, ansi.Reset)
		}

		fmt.Print("\r" + ansi.ClearLine)
		if selected == 1 && editing {
			fmt.Printf("  %s▸ Blocked ports     %s%s_%s\r\n", ansi.Green, ansi.Yellow, editBuf, ansi.Reset)
		} else if selected == 1 {
			fmt.Printf("  %s▸ Blocked ports     %s%s%s\r\n", ansi.Green, ansi.Yellow, portsLabel, ansi.Reset)
		} else {
			fmt.Printf("    %sBlocked ports     %s%s\r\n", ansi.Dim, portsLabel, ansi.Reset)
		}

		fmt.Print("\r" + ansi.ClearLine + "\r\n")

		fmt.Print("\r" + ansi.ClearLine)
		if editErr != "" {
			fmt.Printf("  %s%s✗ %s%s\r\n", ansi.Bold, ansi.Red, editErr, ansi.Reset)
		} else if editing {
			fmt.Printf("  %sType port numbers separated by commas · enter confirm · esc cancel%s\r\n", ansi.Dim, ansi.Reset)
		} else {
			fmt.Printf("  %s↑/↓ navigate · enter select · q save & quit · ctrl+c discard & quit%s\r\n", ansi.Dim, ansi.Reset)
		}
	}

	clearMenu := func() {
		lines := 7
		for i := 0; i < lines; i++ {
			fmt.Printf("\033[A\r" + ansi.ClearLine)
		}
	}

	renderMenu()

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}

		if editing {
			if n == 1 {
				switch buf[0] {
				case 13: // Enter
					editErr = ""
					ports, parseErr := parsePorts(editBuf)
					if parseErr != nil {
						editErr = parseErr.Error()
						clearMenu()
						renderMenu()
						continue
					}
					cfg.Network.BlockedPorts = ports
					editing = false
					editBuf = ""
					clearMenu()
					renderMenu()
				case 27: // Esc
					editing = false
					editBuf = ""
					editErr = ""
					clearMenu()
					renderMenu()
				case 127, 8: // Backspace
					if len(editBuf) > 0 {
						editBuf = editBuf[:len(editBuf)-1]
					}
					editErr = ""
					clearMenu()
					renderMenu()
				default:
					if (buf[0] >= '0' && buf[0] <= '9') || buf[0] == ',' || buf[0] == ' ' {
						editBuf += string(buf[0])
						editErr = ""
						clearMenu()
						renderMenu()
					}
				}
			}
			continue
		}

		if n == 1 {
			switch buf[0] {
			case 'q':
				clearMenu()
				if saveErr := config.Save(cfgPath, cfg); saveErr != nil {
					fmt.Printf("\r%s%s✗ Failed to save: %s%s\r\n", ansi.Bold, ansi.Red, saveErr, ansi.Reset)
					return saveErr
				}
				fmt.Printf("\r%s✓ Config saved to %s%s\r\n", ansi.Green, cfgPath, ansi.Reset)
				return nil
			case 3: // Ctrl+C
				clearMenu()
				fmt.Printf("\r%sDiscarded changes.%s\r\n", ansi.Dim, ansi.Reset)
				return nil
			case 13: // Enter
				if selected == 0 {
					if cfg.Clone.Mode == "shallow" {
						cfg.Clone.Mode = "deep"
					} else {
						cfg.Clone.Mode = "shallow"
					}
					clearMenu()
					renderMenu()
				} else if selected == 1 {
					editing = true
					parts := make([]string, len(cfg.Network.BlockedPorts))
					for i, p := range cfg.Network.BlockedPorts {
						parts[i] = strconv.Itoa(p)
					}
					editBuf = strings.Join(parts, ", ")
					clearMenu()
					renderMenu()
				}
			case 'k':
				if selected > 0 {
					selected--
					clearMenu()
					renderMenu()
				}
			case 'j':
				if selected < items-1 {
					selected++
					clearMenu()
					renderMenu()
				}
			}
		}

		if n == 3 && buf[0] == 27 && buf[1] == 91 {
			switch buf[2] {
			case 65: // Up
				if selected > 0 {
					selected--
					clearMenu()
					renderMenu()
				}
			case 66: // Down
				if selected < items-1 {
					selected++
					clearMenu()
					renderMenu()
				}
			}
		}
	}

	return nil
}

func parsePorts(input string) ([]int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return []int{}, nil
	}

	parts := strings.Split(input, ",")
	ports := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: must be a number", part)
		}
		if err := config.ValidatePorts([]int{p}); err != nil {
			return nil, err
		}
		ports = append(ports, p)
	}
	return ports, nil
}
