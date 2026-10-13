// Command mail_cli is a command-line mail client.
//
// This file is the composition root: it parses just enough of the command line
// to choose a config, builds the backend factory and the services in
// wiring.go, binds them to the interfaces the cli and tui packages declare, and
// gets out of the way. Nothing here does mail work, and nothing below imports
// it.
package main
