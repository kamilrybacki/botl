package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kamilrybacki/botl/internal/ansi"
	"golang.org/x/term"
)

const (
	repoDir   = "/workspace/repo"
	outputDir = "/output"
)

var validSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)

type option struct {
	label string
	desc  string
}

var options = []option{
	{"Push to a remote branch", "Push all commits to a branch on the remote repository"},
	{"Create a git diff patch", "Export uncommitted changes as a .patch file"},
	{"Save workspace to local path", "Copy the entire workspace to the mounted output directory"},
	{"Discard and exit", "Throw away all changes and exit"},
}

func main() {
	// Check if there are any changes at all
	if err := os.Chdir(repoDir); err != nil {
		printError("Cannot access workspace: " + err.Error())
		os.Exit(1)
	}
	hasChanges := checkChanges()

	fmt.Println()
	printBox("botl: Claude Code session complete")
	fmt.Println()

	if !hasChanges {
		printDim("  No changes detected in the workspace.")
		fmt.Println()
		os.Exit(0)
	}

	// Show summary of changes
	printChangeSummary()
	fmt.Println()

	selected := runMenu()
	fmt.Println()

	switch selected {
	case 0:
		handlePush()
	case 1:
		handlePatch()
	case 2:
		handleSave()
	case 3:
		printDim("  Discarding changes.")
		fmt.Println()
	}
}

func checkChanges() bool {
	hasUncommitted := cmdOutput("git", "status", "--porcelain") != ""

	// Check if there are new commits since the clone
	initialHead := os.Getenv("BOTL_INITIAL_HEAD")
	hasNewCommits := false
	if initialHead != "" && validSHARe.MatchString(initialHead) {
		currentHead := cmdOutput("git", "rev-parse", "HEAD")
		hasNewCommits = currentHead != initialHead
	}

	return hasUncommitted || hasNewCommits
}

func printChangeSummary() {
	printBold("  Changes summary:")

	// New commits since clone
	initialHead := os.Getenv("BOTL_INITIAL_HEAD")
	if initialHead != "" && validSHARe.MatchString(initialHead) {
		currentHead := cmdOutput("git", "rev-parse", "HEAD")
		if currentHead != initialHead {
			logOut := cmdOutput("git", "log", "--oneline", initialHead+"..HEAD")
			if logOut != "" {
				lines := strings.Split(strings.TrimSpace(logOut), "\n")
				fmt.Printf("  %s%d new commit(s):%s\n", ansi.Cyan, len(lines), ansi.Reset)
				for _, line := range lines {
					fmt.Printf("    %s%s%s\n", ansi.Dim, line, ansi.Reset)
				}
			}
		}
	}

	// Uncommitted changes
	status := cmdOutput("git", "status", "--porcelain")
	if status != "" {
		lines := strings.Split(strings.TrimSpace(status), "\n")
		fmt.Printf("  %s%d uncommitted file(s) modified%s\n", ansi.Yellow, len(lines), ansi.Reset)
	}
}

func runMenu() int {
	selected := 0

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		// Fallback to simple numbered input
		return fallbackMenu()
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	fmt.Print(ansi.CursorHide)
	defer fmt.Print(ansi.CursorShow)

	renderMenu := func() {
		for i, opt := range options {
			fmt.Print("\r" + ansi.ClearLine)
			if i == selected {
				fmt.Printf("  %s▸ %s%s%s\n", ansi.Green, ansi.Bold, opt.label, ansi.Reset)
			} else {
				fmt.Printf("    %s%s%s\n", ansi.Dim, opt.label, ansi.Reset)
			}
		}
		// Description line
		fmt.Print("\r" + ansi.ClearLine)
		fmt.Printf("  %s%s%s\n", ansi.Dim, options[selected].desc, ansi.Reset)
	}

	clearMenu := func() {
		// Move up and clear all menu lines
		lines := len(options) + 1 // options + description
		for i := 0; i < lines; i++ {
			fmt.Printf("\033[A" + ansi.ClearLine)
		}
	}

	fmt.Printf("  %sUse ↑/↓ arrows, Enter to select:%s\n", ansi.Dim, ansi.Reset)
	renderMenu()

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			break
		}

		if n == 1 {
			switch buf[0] {
			case 13: // Enter
				clearMenu()
				fmt.Print("\r" + ansi.ClearLine)
				fmt.Printf("  %s✓ %s%s\n", ansi.Green, options[selected].label, ansi.Reset)
				return selected
			case 'q', 3: // q or Ctrl+C
				clearMenu()
				fmt.Print("\r" + ansi.ClearLine)
				return len(options) - 1 // discard
			case 'k': // vim up
				if selected > 0 {
					selected--
				}
				clearMenu()
				renderMenu()
			case 'j': // vim down
				if selected < len(options)-1 {
					selected++
				}
				clearMenu()
				renderMenu()
			}
		}

		if n == 3 && buf[0] == 27 && buf[1] == 91 {
			switch buf[2] {
			case 65: // Up
				if selected > 0 {
					selected--
				}
				clearMenu()
				renderMenu()
			case 66: // Down
				if selected < len(options)-1 {
					selected++
				}
				clearMenu()
				renderMenu()
			}
		}
	}

	return selected
}

func fallbackMenu() int {
	fmt.Println("  What would you like to do with the changes?")
	for i, opt := range options {
		fmt.Printf("  [%d] %s\n", i+1, opt.label)
	}
	fmt.Print("  > ")
	var choice int
	fmt.Scanf("%d", &choice)
	if choice < 1 || choice > len(options) {
		return len(options) - 1
	}
	return choice - 1
}

func handlePush() {
	printBold("  Push to remote branch")
	fmt.Println()

	// Stage any uncommitted changes
	status := cmdOutput("git", "status", "--porcelain")
	if status != "" {
		fmt.Print("  Commit message for uncommitted changes: ")
		msg := readLine()
		if msg == "" {
			msg = "botl: uncommitted changes"
		}
		cmdExec("git", "add", "-A")
		cmdExec("git", "commit", "-m", msg)
	}

	// Ask for branch name
	defaultBranch := "botl/" + time.Now().Format("20060102-150405")
	fmt.Printf("  Branch name [%s]: ", defaultBranch)
	branch := readLine()
	if branch == "" {
		branch = defaultBranch
	}

	// Validate branch name
	validBranch := regexp.MustCompile(`^[a-zA-Z0-9._/-]{1,200}$`)
	if !validBranch.MatchString(branch) {
		printError("Invalid branch name: must contain only alphanumeric, dots, slashes, hyphens, underscores")
		os.Exit(1)
	}

	fmt.Printf("  Pushing to %s%s%s...\n", ansi.Cyan, branch, ansi.Reset)
	cmd := exec.Command("git", "push", "origin", "HEAD:"+branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		printError("Push failed: " + err.Error())
		fmt.Println("  You can try pushing manually from inside the container.")
		os.Exit(1)
	}
	fmt.Printf("  %s✓ Pushed to %s%s\n", ansi.Green, branch, ansi.Reset)
}

func handlePatch() {
	printBold("  Creating git diff patch")
	fmt.Println()

	if err := os.MkdirAll(outputDir, 0700); err != nil {
		printError("Cannot access output directory: " + err.Error())
		printDim("  Make sure botl was run with --output-dir")
		os.Exit(1)
	}

	name := "botl-" + time.Now().Format("20060102-150405") + ".patch"
	patchPath := filepath.Join(outputDir, name)

	f, err := os.Create(patchPath)
	if err != nil {
		printError("Failed to create patch file: " + err.Error())
		os.Exit(1)
	}
	defer f.Close()

	// New commits since clone
	initialHead := os.Getenv("BOTL_INITIAL_HEAD")
	if initialHead != "" && validSHARe.MatchString(initialHead) {
		currentHead := cmdOutput("git", "rev-parse", "HEAD")
		if currentHead != initialHead {
			patches := cmdOutput("git", "format-patch", "--stdout", initialHead+"..HEAD")
			if patches != "" {
				f.WriteString(patches)
			}
		}
	}

	// Staged changes
	staged := cmdOutput("git", "diff", "--cached")
	if staged != "" {
		f.WriteString("\n# --- Staged changes ---\n")
		f.WriteString(staged)
	}

	// Unstaged changes
	uncommitted := cmdOutput("git", "diff")
	if uncommitted != "" {
		f.WriteString("\n# --- Unstaged changes ---\n")
		f.WriteString(uncommitted)
	}

	fmt.Printf("  %s✓ Patch saved to %s%s\n", ansi.Green, patchPath, ansi.Reset)
	fmt.Printf("  %s(available at your --output-dir on the host)%s\n", ansi.Dim, ansi.Reset)
}

func handleSave() {
	printBold("  Saving workspace to local path")
	fmt.Println()

	if err := os.MkdirAll(outputDir, 0700); err != nil {
		printError("Cannot access output directory: " + err.Error())
		printDim("  Make sure botl was run with --output-dir")
		os.Exit(1)
	}

	dest := filepath.Join(outputDir, "workspace-"+time.Now().Format("20060102-150405"))

	cmd := exec.Command("cp", "-a", repoDir, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		printError("Failed to copy workspace: " + err.Error())
		os.Exit(1)
	}

	fmt.Printf("  %s✓ Workspace saved to %s%s\n", ansi.Green, dest, ansi.Reset)
	fmt.Printf("  %s(available at your --output-dir on the host)%s\n", ansi.Dim, ansi.Reset)
}

// --- Helpers ---

func readLine() string {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}

func cmdOutput(name string, args ...string) string {
	out, _ := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(out))
}

func cmdExec(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func printBox(text string) {
	w := len(text) + 4
	fmt.Printf("  %s╭%s╮%s\n", ansi.Cyan, strings.Repeat("─", w), ansi.Reset)
	fmt.Printf("  %s│  %s  │%s\n", ansi.Cyan, text, ansi.Reset)
	fmt.Printf("  %s╰%s╯%s\n", ansi.Cyan, strings.Repeat("─", w), ansi.Reset)
}

func printBold(text string) {
	fmt.Printf("%s%s%s\n", ansi.Bold, text, ansi.Reset)
}

func printDim(text string) {
	fmt.Printf("%s%s%s\n", ansi.Dim, text, ansi.Reset)
}

func printError(text string) {
	fmt.Printf("  %s%s✗ %s%s\n", ansi.Bold, ansi.Red, text, ansi.Reset)
}
