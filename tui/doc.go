// Package tui is the interactive front end, built on bubbletea.
//
// Like cli it declares the interfaces it needs and is handed implementations by
// main, so it neither imports runner nor is imported by anything below it. That
// separation is what lets a backend be exercised without starting a terminal
// program, and what stopped the domain layer from having to know whether a TUI
// was running: an app.TuiActive global used to mute printing below the UI, and
// it is gone.
//
// The model holds view state only. Anything that talks to a server happens in a
// tea.Cmd and returns a message, so the update loop is never blocked on the
// network.
package tui
