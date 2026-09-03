// Package scripts hosts repository operational helpers.
//
// Every other .go file in this directory is a standalone admin utility
// guarded by `//go:build ignore`. doc.go keeps the package buildable so
// regression tests (e.g. gate_exitcode_test.go) can live here without
// compiling the ignored utilities.
package scripts
