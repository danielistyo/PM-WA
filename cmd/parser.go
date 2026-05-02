package cmd

import (
	"errors"
	"strings"
)

var ErrEmptyMessage = errors.New("empty message")

type ParsedCommand struct {
	Command string
	Fields  map[string]string
}

func ParseCommand(text string) (*ParsedCommand, error) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return nil, ErrEmptyMessage
	}

	cmd := &ParsedCommand{
		Command: strings.TrimSpace(strings.ToLower(lines[0])),
		Fields:  make(map[string]string),
	}

	for _, line := range lines[1:] {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(strings.ToLower(parts[0]))
			val := strings.TrimSpace(parts[1])
			cmd.Fields[key] = val
		}
	}
	return cmd, nil
}
