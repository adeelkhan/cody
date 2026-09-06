package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"cody/internal/app"
)

func parseArgs(args []string) (path string, nerdFont bool, err error) {
	fs := flag.NewFlagSet("cody", flag.ContinueOnError)
	noNerdFont := fs.Bool("no-nerd-font", false, "use plain ASCII/Unicode icons instead of Nerd Font glyphs")
	if err = fs.Parse(args); err != nil {
		return "", false, err
	}
	path = "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}
	return path, !*noNerdFont, nil
}

func main() {
	rawPath, nerdFont, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path, err := app.ResolveProjectPath([]string{rawPath})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	model, err := app.New(path, nerdFont)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
