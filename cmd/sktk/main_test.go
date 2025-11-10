package main

import (
	"testing"

	"github.com/urfave/cli/v3"
)

func TestCommandStructure(t *testing.T) {
	// Create the main command
	cmd := &cli.Command{
		Name:  "sktk",
		Usage: "Skintrackr CLI - Interact with your Strava data",
		Commands: []*cli.Command{
			loginCommand(),
			exportGpxCommand(),
		},
	}

	// Verify command name
	if cmd.Name != "sktk" {
		t.Errorf("Command name = %v, want %v", cmd.Name, "sktk")
	}

	// Verify subcommands exist
	if len(cmd.Commands) != 2 {
		t.Errorf("Expected 2 subcommands, got %d", len(cmd.Commands))
	}

	// Verify subcommand names
	commandNames := make(map[string]bool)
	for _, subcmd := range cmd.Commands {
		commandNames[subcmd.Name] = true
	}

	if !commandNames["login"] {
		t.Error("Expected 'login' subcommand")
	}
	if !commandNames["export-gpx"] {
		t.Error("Expected 'export-gpx' subcommand")
	}
}

func TestLoginCommand(t *testing.T) {
	cmd := loginCommand()

	if cmd.Name != "login" {
		t.Errorf("Command name = %v, want %v", cmd.Name, "login")
	}

	if cmd.Action == nil {
		t.Error("Login command should have an Action")
	}
}

func TestExportGpxCommand(t *testing.T) {
	cmd := exportGpxCommand()

	if cmd.Name != "export-gpx" {
		t.Errorf("Command name = %v, want %v", cmd.Name, "export-gpx")
	}

	if cmd.Action == nil {
		t.Error("Export-gpx command should have an Action")
	}

	// Verify output flag exists
	hasOutputFlag := false
	for _, flag := range cmd.Flags {
		if stringFlag, ok := flag.(*cli.StringFlag); ok {
			if stringFlag.Name == "output" {
				hasOutputFlag = true
				break
			}
		}
	}

	if !hasOutputFlag {
		t.Error("Export-gpx command should have 'output' flag")
	}
}
