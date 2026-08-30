// Package clioutput defines the stable machine-readable envelope used by
// contributor and consumer CLIs. It deliberately carries no persistent state.
package clioutput

import (
	"encoding/json"
	"io"
)

const SchemaVersion = 1

// Result is the stable JSON contract for one command outcome.
type Result struct {
	Schema      int            `json:"schema"`
	OK          bool           `json:"ok"`
	Command     string         `json:"command"`
	Code        string         `json:"code"`
	What        string         `json:"what"`
	Why         string         `json:"why"`
	Remediation string         `json:"remediation"`
	NextCommand string         `json:"next_command"`
	Data        map[string]int `json:"data,omitempty"`
}

// New returns a complete result with the current schema version.
func New(ok bool, command, code, what, why, remediation, nextCommand string) Result {
	return Result{
		Schema:      SchemaVersion,
		OK:          ok,
		Command:     command,
		Code:        code,
		What:        what,
		Why:         why,
		Remediation: remediation,
		NextCommand: nextCommand,
	}
}

// Write emits exactly one JSON object followed by a newline.
func Write(writer io.Writer, result Result) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result)
}
