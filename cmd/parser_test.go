package cmd

import (
	"testing"
)

// TestParseCommandAddTaskShape locks the single-task command format: the bulk
// command does not share this code path, and add-task behavior must be
// unaffected by the add-tasks addition.
func TestParseCommandAddTaskShape(t *testing.T) {
	text := "add-task\nwa-group: G\nlist: L\ntitle: Things: apart\nassign: 6281234567890\ndeadline: 2026-11-06 17:00"
	parsed, err := ParseCommand(text)
	if err != nil {
		t.Fatalf("ParseCommand() unexpected error: %v", err)
	}
	if parsed.Command != "add-task" {
		t.Fatalf("Command = %q, want add-task", parsed.Command)
	}
	if got := parsed.Fields["title"]; got != "Things: apart" {
		t.Fatalf("Fields[title] = %q, want %q", got, "Things: apart")
	}
	if parsed.Fields["wa-group"] != "G" || parsed.Fields["list"] != "L" ||
		parsed.Fields["assign"] != "6281234567890" || parsed.Fields["deadline"] != "2026-11-06 17:00" {
		t.Fatalf("Fields = %v, want single-task shape preserved", parsed.Fields)
	}
}

func TestParseCommandSingleTaskMultipleAssignees(t *testing.T) {
	parsed, err := ParseCommand("add-task\nwa-group: G\nlist: L\ntitle: A\nassign: 6281234567890, 6289876543210\ndeadline: 2026-11-06 17:00\nreminder: no")
	if err != nil {
		t.Fatalf("ParseCommand() unexpected error: %v", err)
	}
	if got := parsed.Fields["assign"]; got != "6281234567890, 6289876543210" {
		t.Fatalf("Fields[assign] = %q, want both phones in one field", got)
	}
	if parsed.Fields["reminder"] != "no" {
		t.Fatalf("Fields[reminder] = %q, want 'no'", parsed.Fields["reminder"])
	}
}

func TestParseCommandNormalizesCommandWord(t *testing.T) {
	parsed, err := ParseCommand("  ADD-TASK  \ntitle: A")
	if err != nil {
		t.Fatalf("ParseCommand() unexpected error: %v", err)
	}
	if parsed.Command != "add-task" {
		t.Fatalf("Command = %q, want 'add-task'", parsed.Command)
	}
}
