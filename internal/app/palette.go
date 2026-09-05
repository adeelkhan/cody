package app

import "strings"

func filteredCommands(commands []Command, query string) []Command {
	if query == "" {
		return commands
	}
	q := strings.ToLower(query)
	var out []Command
	for _, c := range commands {
		if strings.Contains(strings.ToLower(c.Name), q) {
			out = append(out, c)
		}
	}
	return out
}
