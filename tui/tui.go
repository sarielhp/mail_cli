package tui

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/calendar"
	"github.com/sarielhp/mail_cli/cfg_g"
	"github.com/sarielhp/mail_cli/email"
	"github.com/sarielhp/mail_cli/filebrowser"
	"github.com/sarielhp/mail_cli/label"
	"github.com/sarielhp/mail_cli/llm"
	"github.com/sarielhp/mail_cli/mailclient"
	"github.com/sarielhp/mail_cli/report"
	"github.com/sarielhp/mail_cli/tui/render"

	"github.com/charmbracelet/bubbles/viewport"
)

const modeIndex int = 0
const modeDetail = 1
const modeCompose = 2

// Header column widths for email row rendering
const (
	HeaderColIndexWidth   = 4  // Index column width
	HeaderColDateWidth    = 8  // Date column width
	HeaderColFromWidth    = 20 // From column width
	HeaderColSubjectWidth = 2  // Subject column width
	HeaderColSpamWidth    = 2  // Spam indicator width
	HeaderColReadWidth    = 5  // Status and attachment indicators width
	HeaderColPadding      = 2  // Padding between columns
)

const (
	headerColTotal = HeaderColIndexWidth + HeaderColDateWidth + HeaderColFromWidth + HeaderColSubjectWidth + HeaderColSpamWidth + HeaderColReadWidth + (HeaderColPadding * 5)
)

type pendingOpType int

const (
	pendingSpam    pendingOpType = 1
	pendingArchive pendingOpType = 2
	pendingDelete  pendingOpType = 3
	pendingUnspam  pendingOpType = 4
)

type timeTickMsg struct{}

type errMsg struct {
	err error
}

type foldersLoadedMsg struct {
	folders []label.Item
}

type emailsLoadedMsg struct {
	emails      []email.Email
	classifyIDs []email.ID
	// Update. Assigning them to the model from inside the goroutine raced the
	// key handlers that write the same maps, which can abort the process with a
	// non-recoverable "concurrent map writes" fatal error.
	isRead    map[email.ID]bool
	isReplied map[email.ID]bool
}

type detailMsg struct {
	headers     string
	body        string
	attachments []email.Attachment
	crypto      *email.CryptoStatus
}

type doneMsg struct {
	id email.ID
}

type spamDoneMsg struct {
	id email.ID
}

type archiveDoneMsg struct {
	id email.ID
}

type deleteDoneMsg struct {
	id email.ID
}

type unspamDoneMsg struct {
	id email.ID
}

// markedSpamDoneMsg reports the outcome of moving [SPAM] messages to the spam
// folder. Only the moved ids are dropped from the index; failed maps each id
// that stayed put to the reason.
type markedSpamDoneMsg struct {
	ids    []email.ID
	failed map[email.ID]error
}

// spamClassifiedMsg reports whether a message was learned as spam. On success
// it carries the [SPAM] badge in place; on failure err is set and the optimistic
// badge is rolled back.
type spamClassifiedMsg struct {
	id  email.ID
	err error
}

type classifyDoneMsg struct{}

type clearSpamUndoMsg struct {
	time time.Time
}

type deadlineEntry struct {
	EmailID     email.ID
	Sender      string
	Subject     string
	DateText    string
	Description string
	EmailDate   time.Time
}

func (m *tuiModel) isPending(id email.ID) bool {
	_, ok := m.pendingEmails[id]
	return ok
}

func (m *tuiModel) addPending(id email.ID, op pendingOpType) {
	m.pendingEmails[id] = op
}

func (m *tuiModel) removePending(id email.ID) {
	delete(m.pendingEmails, id)
}

type tuiModel struct {
	mode               int
	folders            []label.Item
	emails             []email.Email
	rawEmails          []email.Email
	expandedThreads    map[threadKey]bool
	eIdx               int
	tree               []*label.TreeNode
	entries            []label.TreeEntry
	cursor             int
	currentF           string
	treeOpen           bool
	showHelp           bool
	showLoad           string
	vp                 viewport.Model
	done               bool
	width              int
	height             int
	lpr                string
	selectedID         email.ID
	client             mailclient.MailClient
	pendingEmails      map[email.ID]pendingOpType
	detailH            string
	detail             string
	detailCrypto       *email.CryptoStatus
	selectedRow        render.ColoredString
	detailVpDirty      bool
	theme              *render.ThemeManager
	lastTime           time.Time
	folderSearch       string
	inFolderSearch     bool
	inSearch           bool
	searchQuery        string
	fuzzySearch        bool
	fuzzyGlobal        bool
	savedRawEmails     []email.Email
	savedCurrentF      string
	detailSearch       string
	inDetailSearch     bool
	inPipePrompt       bool
	pipeCommand        string
	taggedIDs          map[email.ID]bool
	inTagPrefix        bool
	inTagPatternPrompt bool
	tagPatternQuery    string
	bulkMovingToFolder bool
	inRemoteSearch     bool
	remoteSearchQuery  string
	splitPreview       bool
	indexStart         int
	err                error
	globalFolders      []label.Item
	globalTree         []*label.TreeNode
	showGlobalTree     bool
	folderJustChanged  bool
	isRead             map[email.ID]bool
	isReplied          map[email.ID]bool
	bk                 *Backend
	rep                report.Reporter
	showError          bool
	showDiag           bool
	diagVp             viewport.Model
	attachments        []email.Attachment
	attachmentDrawer   bool
	attCursor          int
	saveStatus         string
	// exitNotice is printed once the program has exited, for an exit that
	// saved a draft: where the local copy is and whether the upload landed.
	exitNotice string
	// draftSyncs is the server-side draft work of each compose session,
	// keyed by its Message-ID; it outlives the session (tui_compose_draft.go).
	draftSyncs map[string]*draftSync
	// exitPending is set while the program waits for a session's server work
	// before exiting; abandonArmed is the first Ctrl+C of an abandon.
	exitPending bool
	// contactPick is the open contacts picker (F5 on the compose screen).
	contactPick    *contactPicker
	abandonArmed   bool
	saveStatusTime time.Time
	// notice is a transient confirmation shown under the index and detail
	// views, such as where F2 saved a message.
	notice     string
	noticeTime time.Time
	menuOpen   bool
	menuCursor int
	menuItems  []string
	// spamMenuOpen is the F5 spam dialog in the index view; spamMenuCursor is
	// its selection.
	spamMenuOpen       bool
	spamMenuCursor     int
	accountOverlayOpen bool
	accountCursor      int
	cfg                *cfg_g.Config
	showSummary        bool
	summaryContent     string
	summaryEmailID     email.ID
	summaryFrom        string
	summarySubject     string
	summaryDate        string
	summaryVp          viewport.Model
	summaryCache       map[email.ID]string
	attachmentHelpOpen bool
	fileBrowserOpen    bool
	fileBrowser        filebrowser.Model
	pendingSaveAtt     email.Attachment
	// pendingSaveStem is set when the save dialog is saving a whole message
	// (F2): its name is shortened against the directory finally chosen.
	pendingSaveStem           string
	lastSpammedID             email.ID
	lastSpammedTime           time.Time
	undoSpamBanner            string
	summaryQueue              []email.ID
	summaryInFlight           bool
	currentSummarizingID      email.ID
	waitingUserSummaryID      email.ID
	savingMessageToFolder     bool
	savingMessageID           email.ID
	savingMessageSourceFolder string
	summaryParsedCache        map[email.ID]llm.ParsedSummary
	showDeadlinesAgenda       bool
	deadlinesVp               viewport.Model
	deadlinesCursor           int
	deadlinesList             []deadlineEntry
	icsModalOpen              bool
	icsFilename               string
	icsData                   []byte
	icsEvent                  *calendar.ICSEvent
	icsEvents                 []*calendar.ICSEvent
	icsEventIdx               int
	icsCheckStatus            icsCheckStatus
	blockModalOpen            bool
	blockSenderName           string
	blockSenderEmail          string
	blockMessageID            email.ID
	icsInCalendar             bool
	icsDiscrepancy            string
	icsCheckErr               string
	icsAddStatus              string
	icsVp                     viewport.Model
	attModalOpen              bool
	attPreviewAtt             email.Attachment
	attPreviewVp              viewport.Model
	attPreviewStatus          string
	attPreviewStatusTime      time.Time
	attInvertColors           bool
	attachmentLineOffset      int
	detailSplitOpen           bool
	rightPaneFocus            bool
	hideRightPane             bool
	rightVp                   viewport.Model
	rightVpAttFilename        string
	attachmentListOpen        bool
	attachmentViewerOpen      bool
	attachmentViewerVp        viewport.Model
	attachmentDeleted         map[int]bool
	attachmentListCursor      int
	compose                   *composeState
	standaloneCompose         bool
	// mouseOff is set while F9 has handed the mouse back to the terminal.
	mouseOff bool
	// lastFrame is the screen as last drawn, and frameKind what it was -- the
	// index or the reader with nothing over it (frameBase), or which overlay;
	// a click is read against them. See tui_mouse_overlay.go.
	lastFrame string
	frameKind string
	// summaryShown is the summary as last put in its viewport; see
	// setupSummaryViewport.
	summaryShown  string
	composeOpts   ComposeOptions
	lockedFolders map[string]bool
}

func newTuiModel(cl mailclient.MailClient, lp string, bk *Backend) *tuiModel {
	// rep is replaced with the program reporter once the TUI is running.
	m := &tuiModel{
		mode:            modeIndex,
		client:          cl,
		lpr:             lp,
		theme:           render.NewThemeManager(),
		expandedThreads: make(map[threadKey]bool),
		pendingEmails:   make(map[email.ID]pendingOpType),
		lockedFolders:   make(map[string]bool),
		showGlobalTree:  true,
		bk:              bk,
		menuItems:       []string{"Move to Spam", "Switch Account", "Exit"},
	}
	if cl != nil && cl.Config() != nil {
		m.isRead = cache.LoadReadState(cl.Config().AccountCache)
		m.isReplied = cache.LoadRepliedState(cl.Config().AccountCache)
		m.attInvertColors = cl.Config().AttachmentInvertColors
	}
	if m.isRead == nil {
		m.isRead = make(map[email.ID]bool)
	}
	if m.isReplied == nil {
		m.isReplied = make(map[email.ID]bool)
	}
	m.vp = viewport.New(70, 20)
	m.diagVp = viewport.New(70, 20)
	m.summaryVp = viewport.New(70, 20)
	m.icsVp = viewport.New(70, 8)
	m.summaryCache = make(map[email.ID]string)
	m.summaryParsedCache = make(map[email.ID]llm.ParsedSummary)
	m.deadlinesVp = viewport.New(70, 20)
	targetFolder := "INBOX"
	if cl != nil {
		targetFolder = cl.InboxFolder().String()
	}
	if lp != "" {
		targetFolder = lp
	}
	slog.Info("newTuiModel initializing", slog.String("labelPrefix", lp), slog.String("targetFolderDefault", targetFolder))

	var cachedFolders []label.Item
	if cl != nil && cl.Config() != nil {
		dir := cl.Config().AccountCache
		if !dir.IsZero() {
			cachePath := filepath.Join(dir.Path(), "labels_cache.json")
			slog.Info("newTuiModel loading cached folders", slog.String("path", cachePath))
			if data, err := os.ReadFile(cachePath); err == nil {
				if unmarshalErr := json.Unmarshal(data, &cachedFolders); unmarshalErr != nil {
					slog.Error("newTuiModel failed to unmarshal cached folders", slog.Any("error", unmarshalErr))
				} else {
					var names []string
					for _, f := range cachedFolders {
						names = append(names, f.FullName)
					}
					slog.Info("newTuiModel loaded cached folders successfully", slog.Int("count", len(cachedFolders)), slog.Any("folderNames", names))
				}
			} else {
				slog.Warn("newTuiModel failed to read cached folders file", slog.Any("error", err))
			}
		}
	}
	if len(cachedFolders) > 0 {
		m.globalFolders = cachedFolders
		m.globalTree = label.BuildLabelTree(cachedFolders)
		if lp != "" {
			lower := strings.ToLower(lp)
			var filtered []label.Item
			for _, f := range cachedFolders {
				if strings.Contains(strings.ToLower(f.FullName), lower) {
					filtered = append(filtered, f)
				}
			}
			slog.Info("newTuiModel filtered cached folders by prefix", slog.Int("filteredCount", len(filtered)), slog.String("prefix", lp))
			cachedFolders = filtered
		}
	}
	if len(cachedFolders) > 0 {
		m.folders = cachedFolders
		m.tree = label.BuildLabelTree(cachedFolders)
		m.buildEntries()
		resolved := label.ResolveFolder(cachedFolders, targetFolder, "INBOX")
		slog.Info("newTuiModel resolved target folder from cache", slog.String("resolvedFolder", resolved), slog.Bool("found", resolved != ""))
		m.currentF = resolved
	} else {
		slog.Warn("newTuiModel found no cached folders; defaulting directly to INBOX")
		m.currentF = targetFolder
	}
	m.loadCachedEmailsForFolder(m.currentF)
	return m
}
