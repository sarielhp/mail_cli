package app

// The bracketed markers that lead a status line -- "[*]", "[+]", "[!]" -- in
// the colour of the message they introduce.
var (
	PrefixInfo    = ColorBoldCyan.Sprint("[*]")
	PrefixSuccess = ColorBoldGreen.Sprint("[+]")
	PrefixWarn    = ColorBoldYellow.Sprint("[!]")
	PrefixError   = ColorBoldRed.Sprint("[!]")
)
