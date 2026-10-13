// Package calendar reads the .ics attachments that arrive as meeting
// invitations, turning them into an ICSEvent the rest of the program can show
// or file.
//
// Times are resolved through the IANA database: an explicit ...Z value is UTC,
// and a TZID that time.LoadLocation knows is parsed in that location. A TZID it
// does not know leaves the wall-clock time read as UTC and ZoneUnresolved set,
// because the resulting instant is wrong by that zone's offset and a caller
// filing a real calendar entry needs to know it is a guess.
package calendar
