//go:build windows

package terminal

import "os/exec"

// setControllingTerminal is a no-op on Windows — xpty's ConPTY path
// handles the child's console attachment differently and needs no
// equivalent of Unix's session-leader/controlling-terminal setup.
func setControllingTerminal(cmd *exec.Cmd) {}
