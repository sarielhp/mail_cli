package interactive

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"
)

const envVar = "MAIL_CLI_INTERACTIVE"

// requiredTool is a program the harness runs, and the Debian package that
// provides it.
type requiredTool struct {
	bin, pkg, why string
}

var requiredTools = []requiredTool{
	{"xvfb-run", "xvfb", "runs the terminal on a private display"},
	{"Xvfb", "xvfb", "the private display itself"},
	{"kitty", "kitty", "the terminal under test"},
	{"xdotool", "xdotool", "sends real mouse clicks"},
	{"import", "imagemagick", "saves a screenshot when a test fails"},
	{"w3m", "w3m", "renders HTML mail"},
	{"go", "golang-go", "builds the binary under test"},
}

// requireInteractive skips unless interactive tests were asked for, and then
// fails -- naming what to install -- when any tool is missing, so a run that
// was asked for never passes by testing nothing.
func requireInteractive(t *testing.T) {
	t.Helper()
	if os.Getenv(envVar) != "1" {
		t.Skipf("interactive test; run with %s=1", envVar)
	}
	var missing []string
	pkgs := map[string]bool{}
	var install []string
	for _, tool := range requiredTools {
		if _, err := exec.LookPath(tool.bin); err == nil {
			continue
		}
		missing = append(missing, fmt.Sprintf("  %-9s (%s)", tool.bin, tool.why))
		if !pkgs[tool.pkg] {
			pkgs[tool.pkg] = true
			install = append(install, tool.pkg)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("interactive tests need tools that are not installed:\n%s\nPlease install them:\n  sudo apt install %s",
			strings.Join(missing, "\n"), strings.Join(install, " "))
	}
}

var (
	buildOnce sync.Once
	binPath   string
	errBuild  error
)

// TestMain removes the binary built for the run.
func TestMain(m *testing.M) {
	code := m.Run()
	if binPath != "" {
		_ = os.RemoveAll(filepath.Dir(binPath))
	}
	os.Exit(code)
}

// binary builds mail_cli once per run, from the module root.
func binary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "mail_cli-interactive-bin")
		if err != nil {
			errBuild = err
			return
		}
		binPath = filepath.Join(dir, "mail_cli")
		cmd := exec.Command("go", "build", "-o", binPath, ".")
		cmd.Dir = ".."
		if out, err := cmd.CombinedOutput(); err != nil {
			errBuild = fmt.Errorf("go build: %w\n%s", err, out)
		}
	})
	if errBuild != nil {
		t.Fatal(errBuild)
	}
	return binPath
}

// session is one mail_cli run in kitty on a private display.
type session struct {
	t      *testing.T
	dir    string
	sock   string
	opened string
	env    []string // DISPLAY and XAUTHORITY, for xdotool and import
}

// launcher runs inside xvfb-run: it records the display it was given, then
// becomes kitty. kitty reads no config file, so cell geometry is the same on
// every machine.
const launcher = `#!/bin/sh
printf 'DISPLAY=%s\nXAUTHORITY=%s\n' "$DISPLAY" "$XAUTHORITY" > "$1"
sock=$2
shift 2
exec kitty --config NONE -o allow_remote_control=yes --listen-on "$sock" \
  -o font_size=11 -o remember_window_size=no \
  -o initial_window_width=1000 -o initial_window_height=800 "$@"
`

// start runs mail_cli with args in a fresh terminal and returns once kitty
// answers remote control. Everything it started is stopped when t ends.
func start(t *testing.T, args ...string) *session {
	t.Helper()
	return startWith(t, nil, args...)
}

// startWith is start with accounts as the config's accounts list.
func startWith(t *testing.T, accounts []map[string]any, args ...string) *session {
	t.Helper()
	requireInteractive(t)
	bin := binary(t)
	s := &session{t: t, dir: t.TempDir()}
	s.sock = filepath.Join(s.dir, "kitty.sock")
	s.opened = filepath.Join(s.dir, "opened.txt")
	home := s.writeHome(accounts)

	launch := filepath.Join(s.dir, "launch.sh")
	if err := os.WriteFile(launch, []byte(launcher), 0o700); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(s.dir, "display.env")
	cmdArgs := append([]string{"-a", "-s", "-screen 0 1280x1024x24", launch, envFile, "unix:" + s.sock, bin}, args...)
	cmd := exec.Command("xvfb-run", cmdArgs...)
	cmd.Env = cleanEnv(home, s.dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	log, err := os.Create(filepath.Join(s.dir, "xvfb.log"))
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatalf("start xvfb-run: %v", err)
	}
	// Cleanups run last-registered first: the screenshot, then stopping the
	// process group, then the check that nothing escaped it.
	t.Cleanup(s.reapStrays)
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		_ = cmd.Wait()
		_ = log.Close()
	})
	t.Cleanup(s.saveOnFailure)
	s.waitReady(envFile)
	return s
}

// writeHome makes the HOME the binary sees: a config whose download_dir is
// inside the test directory, whose browser only records its argument, and
// whose accounts are the given ones -- in a test, fakes on 127.0.0.1.
func (s *session) writeHome(accounts []map[string]any) string {
	home := filepath.Join(s.dir, "home")
	cfgDir := filepath.Join(home, ".config", "mail_cli")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		s.t.Fatal(err)
	}
	browser := filepath.Join(s.dir, "browser")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> '" + s.opened + "'\n"
	if err := os.WriteFile(browser, []byte(script), 0o700); err != nil {
		s.t.Fatal(err)
	}
	if accounts == nil {
		accounts = []map[string]any{}
	}
	cfg, err := json.Marshal(map[string]any{
		"download_dir": filepath.Join(s.dir, "cache"),
		"browser":      browser,
		"accounts":     accounts,
	})
	if err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), cfg, 0o600); err != nil {
		s.t.Fatal(err)
	}
	return home
}

// cleanEnv is the whole environment of the run: nothing of the user's
// session leaks in -- not WAYLAND_DISPLAY, which would put kitty on the
// desktop, and not credentials such as GMAIL_USER.
//
// The session bus points at a socket that does not exist. Left unset, libdbus
// autolaunched a private bus on the Xvfb display, and through it the desktop
// started gvfs -- which mounted FUSE inside the test directory -- the
// accessibility bus and a notification waiter, all of which outlived the test.
func cleanEnv(home, dir string) []string {
	runtime := filepath.Join(dir, "run")
	_ = os.MkdirAll(runtime, 0o700)
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"USER=" + os.Getenv("USER"),
		"LANG=C.UTF-8",
		"XDG_RUNTIME_DIR=" + runtime,
		"LIBGL_ALWAYS_SOFTWARE=1",
		"DBUS_SESSION_BUS_ADDRESS=unix:path=" + filepath.Join(dir, "no-bus"),
		"NO_AT_BRIDGE=1",
		"GIO_USE_VFS=local",
		"GVFS_DISABLE_FUSE=1",
	}
}

// waitReady waits for the display to be recorded and kitty to answer.
func (s *session) waitReady(envFile string) {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(envFile)
		if err == nil && s.kittyAnswers() {
			s.env = strings.Fields(string(data))
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	log, _ := os.ReadFile(filepath.Join(s.dir, "xvfb.log"))
	s.t.Fatalf("kitty did not start within 20s; its output:\n%s", log)
}

func (s *session) kittyAnswers() bool {
	return exec.Command("kitty", "@", "--to", "unix:"+s.sock, "ls").Run() == nil
}

// kitty runs a remote-control command against the session's window.
func (s *session) kitty(args ...string) string {
	s.t.Helper()
	full := append([]string{"@", "--to", "unix:" + s.sock}, args...)
	out, err := exec.Command("kitty", full...).CombinedOutput()
	if err != nil {
		s.t.Fatalf("kitty %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// x runs an X client (xdotool, import) on the session's display.
func (s *session) x(name string, args ...string) string {
	s.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), s.env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		s.t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

// Text is the screen as plain text, one line per row.
func (s *session) Text() string {
	return s.kitty("get-text", "--match", "id:1")
}

// Key presses one key, in kitty's names: "enter", "q", "ctrl+c".
func (s *session) Key(name string) {
	s.kitty("send-key", "--match", "id:1", name)
}

// Type sends text as typed. Send one character at a time: a longer string
// can arrive as a single key event. A shifted letter goes this way, because
// send-key "shift+e" reached the program as "e".
func (s *session) Type(text string) {
	s.kitty("send-text", "--match", "id:1", "--", text)
}

// WaitFor waits until the screen shows want and returns the screen.
func (s *session) WaitFor(want string) string {
	s.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		screen := s.Text()
		if strings.Contains(screen, want) {
			return screen
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("screen never showed %q; last screen:\n%s", want, screen)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Find returns the cell where want first appears on screen. Columns count
// runes, which is the cell column for the narrow text tests look for.
func (s *session) Find(want string) (col, row int) {
	s.t.Helper()
	for row, line := range strings.Split(s.Text(), "\n") {
		if before, _, ok := strings.Cut(line, want); ok {
			return utf8.RuneCountInString(before), row
		}
	}
	s.t.Fatalf("%q is not on screen", want)
	return 0, 0
}

// Click sends a real left click to the middle of cell (col, row). The cell
// size is the window's pixel size over its size in cells.
func (s *session) Click(col, row int) {
	s.t.Helper()
	win := strings.TrimSpace(s.x("xdotool", "search", "--class", "kitty"))
	win = win[strings.LastIndex(win, "\n")+1:]
	w, h := s.windowPixels(win)
	cols, lines := s.windowCells()
	cw, ch := w/cols, h/lines
	x, y := col*cw+cw/2, row*ch+ch/2
	s.x("xdotool", "mousemove", "--window", win, strconv.Itoa(x), strconv.Itoa(y), "click", "1")
}

func (s *session) windowPixels(win string) (w, h int) {
	s.t.Helper()
	for kv := range strings.FieldsSeq(s.x("xdotool", "getwindowgeometry", "--shell", win)) {
		k, v, _ := strings.Cut(kv, "=")
		n, _ := strconv.Atoi(v)
		switch k {
		case "WIDTH":
			w = n
		case "HEIGHT":
			h = n
		}
	}
	if w == 0 || h == 0 {
		s.t.Fatalf("no geometry for window %s", win)
	}
	return w, h
}

func (s *session) windowCells() (cols, lines int) {
	s.t.Helper()
	var osWindows []struct {
		Tabs []struct {
			Windows []struct {
				Columns int `json:"columns"`
				Lines   int `json:"lines"`
			} `json:"windows"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal([]byte(s.kitty("ls")), &osWindows); err != nil {
		s.t.Fatalf("parse kitty ls: %v", err)
	}
	for _, osw := range osWindows {
		for _, tab := range osw.Tabs {
			for _, win := range tab.Windows {
				return win.Columns, win.Lines
			}
		}
	}
	s.t.Fatal("kitty reports no window")
	return 0, 0
}

// WaitOpened waits for the browser to have been asked to open n URLs and
// returns them in order.
func (s *session) WaitOpened(n int) []string {
	s.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, _ := os.ReadFile(s.opened)
		urls := strings.Fields(string(data))
		if len(urls) >= n {
			return urls
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("browser was asked to open %d URL(s), want %d: %q", len(urls), n, urls)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// reapStrays fails the test for, and kills, any process still running with
// the test directory in its environment: a daemon the run started outside
// its process group, which stopping the group did not reach.
func (s *session) reapStrays() {
	time.Sleep(500 * time.Millisecond)
	procs, _ := filepath.Glob("/proc/[0-9]*/environ")
	var strays []string
	for _, path := range procs {
		env, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(env), s.dir) {
			continue
		}
		pidDir := filepath.Dir(path)
		pid, _ := strconv.Atoi(filepath.Base(pidDir))
		cmdline, _ := os.ReadFile(filepath.Join(pidDir, "cmdline"))
		strays = append(strays, fmt.Sprintf("%d %s", pid, strings.ReplaceAll(string(cmdline), "\x00", " ")))
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	if len(strays) > 0 {
		s.t.Errorf("processes outlived the session (now killed):\n  %s", strings.Join(strays, "\n  "))
	}
	// Nothing is mounted unless something leaked, so a failure here is normal.
	_ = exec.Command("fusermount", "-u", filepath.Join(s.dir, "run", "gvfs")).Run()
}

// saveOnFailure keeps a screenshot and the screen text of a failed test,
// outside the test's temporary directory, which is about to be removed.
func (s *session) saveOnFailure() {
	if !s.t.Failed() || s.env == nil {
		return
	}
	out := filepath.Join(os.TempDir(), "mail_cli-interactive", strings.ReplaceAll(s.t.Name(), "/", "_"))
	if err := os.MkdirAll(out, 0o700); err != nil {
		return
	}
	shot := exec.Command("import", "-window", "root", filepath.Join(out, "screen.png"))
	shot.Env = append(os.Environ(), s.env...)
	_ = shot.Run()
	if text, err := exec.Command("kitty", "@", "--to", "unix:"+s.sock, "get-text", "--match", "id:1").Output(); err == nil {
		_ = os.WriteFile(filepath.Join(out, "screen.txt"), text, 0o600)
	}
	s.t.Logf("failure screenshot and screen text saved in %s", out)
}
