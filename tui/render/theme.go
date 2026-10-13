package render

// ColorID names one colour slot in a Theme.
type ColorID int

// These constants name the colour slots a Theme provides; ColorDefault is the
// unset marker that WithFg and WithBg replace.
const (
	ColorDefault ColorID = iota - 1
	ColorBg
	ColorFg
	ColorDate
	ColorSender
	ColorSubject
	ColorHighlightBg
	ColorHighlightFg
	ColorDim
	ColorBorder
	ColorTopBg
	ColorMsgEndBg
	ColorQuote2
	ColorQuote3
	ColorSpam
	ColorPoli
	ColorICS
	// ColorAccount0 is the first of accountColors slots that tell accounts
	// apart in a listing that mixes them.
	ColorAccount0
)

// accountColors is how many account colours a Theme provides.
const accountColors = 6

// AccountColor is the colour slot for the account at position idx in the
// configuration; the slots repeat past the last one.
func AccountColor(idx int) ColorID {
	if idx < 0 {
		idx = -idx
	}
	return ColorAccount0 + ColorID(idx%accountColors)
}

// Theme maps each ColorID to a terminal colour, either a "#rrggbb" hex string
// or a 256-colour palette index.
type Theme struct {
	Colors [int(ColorAccount0) + accountColors]string
}

// Get returns the colour registered for id, or the foreground colour when id is
// out of range.
func (t *Theme) Get(id ColorID) string {
	if int(id) < 0 || int(id) >= len(t.Colors) {
		return t.Colors[ColorFg]
	}
	return t.Colors[id]
}

// ThemeManager holds the active Theme and any named alternates that Switch can
// select.
type ThemeManager struct {
	current *Theme
	names   map[string]*Theme
}

// NewDefaultTheme returns the built-in theme used until another is selected.
func NewDefaultTheme() *Theme {
	return &Theme{
		Colors: [int(ColorAccount0) + accountColors]string{
			"#2b2b2b",
			"#f8f8f2",
			"#888888",
			"#66ccff",
			"#ffffff",
			"#cc2222",
			"#ffdd44",
			"#777777",
			"#4a9e6e",
			"4",
			"92",
			"#336699",
			"#1a3a55",
			"#ff6666",
			"#ff7f7f",
			"#0a2a0a",
			// Account colours: distinct hues, all readable on the dark background.
			"#66d9ef",
			"#f0a35e",
			"#a6e22e",
			"#c792ea",
			"#f78c9c",
			"#e6db74",
		},
	}
}

// NewThemeManager returns a manager holding the default theme and no named
// alternates.
func NewThemeManager() *ThemeManager {
	return &ThemeManager{
		current: NewDefaultTheme(),
		names:   make(map[string]*Theme),
	}
}

// Theme returns the active theme, creating the default one if none has been
// set.
func (tm *ThemeManager) Theme() *Theme {
	if tm.current == nil {
		tm.current = NewDefaultTheme()
	}
	return tm.current
}

// Switch makes the named theme current; an unknown name is ignored and the
// current theme is left unchanged.
func (tm *ThemeManager) Switch(name string) {
	if t, ok := tm.names[name]; ok {
		tm.current = t
	}
}
