package main

import "os/exec"

// duckdbCommand builds an exec.Cmd that emits query results as JSONL on stdout.
func duckdbCommand(sql string) *exec.Cmd {
	return exec.Command("duckdb",
		"-no-stdin",
		"-s", ".mode jsonlines",
		"-s", sql,
	)
}

func newCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
