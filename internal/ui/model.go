package ui

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/0xbenc/dangit/internal/clipboard"
	"github.com/0xbenc/dangit/internal/fuzzy"
	"github.com/0xbenc/dangit/internal/scan"
	"github.com/0xbenc/dangit/internal/termstyle"
)

type phase int

const (
	phaseScanning phase = iota
	phaseBrowsing
)

const spinnerInterval = 110 * time.Millisecond

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Messages streamed from the scan goroutine and async actions.
type (
	progressMsg struct {
		done, total int
		current     string
	}
	scanDoneMsg struct {
		results []scan.Result
		err     error
	}
	spinnerTickMsg struct{}
	detailMsg      struct {
		abs  string
		text string
	}
	planMsg struct {
		abs  string
		plan scan.ResolvePlan
		err  error
	}
	resolveDoneMsg struct {
		abs string
		res scan.ResolveResult
	}
	refreshMsg struct {
		abs string
		res scan.Result
		ok  bool
	}
	execDoneMsg struct {
		abs string
		err error
	}
)

type confirmState struct {
	abs  string
	plan scan.ResolvePlan
}

type model struct {
	ctx   context.Context
	opts  BrowseOptions
	theme termstyle.Theme

	phase         phase
	width, height int

	// scanning
	scanCh  chan tea.Msg
	done    int
	total   int
	current string
	frame   int

	// results
	flagged  []scan.Result
	resolved map[string]bool
	failed   map[string]string

	// browsing
	query     string
	filtering bool
	filtered  []int
	cursor    int
	offset    int

	// overlays
	showDetail bool
	detailText string
	confirm    *confirmState
	working    bool

	notice    string
	noticeErr bool

	quit bool
}

func newModel(ctx context.Context, opts BrowseOptions) model {
	theme := opts.Theme
	if opts.NoColor {
		theme = theme.WithNoColor(true)
	}
	return model{
		ctx:      ctx,
		opts:     opts,
		theme:    theme,
		phase:    phaseScanning,
		scanCh:   make(chan tea.Msg, 256),
		resolved: make(map[string]bool),
		failed:   make(map[string]string),
		width:    80,
		height:   24,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tea.RequestWindowSize, m.startScan(), spinnerTick())
}

// startScan launches the concurrent scan, streaming progress over scanCh.
func (m model) startScan() tea.Cmd {
	ch := m.scanCh
	ctx := m.ctx
	opts := m.opts
	go func() {
		results, err := scan.Scan(ctx, scan.Options{
			Root:      opts.Root,
			Timeout:   opts.Timeout,
			NoNetwork: opts.NoNetwork,
			Env:       opts.Env,
			Progress: func(done, total int, current string) {
				// Cosmetic: drop updates the UI hasn't drained rather than
				// throttling the scan.
				select {
				case ch <- progressMsg{done: done, total: total, current: current}:
				default:
				}
			},
		})
		select {
		case ch <- scanDoneMsg{results: results, err: err}:
		case <-ctx.Done():
		}
	}()
	return waitForScan(ch)
}

func waitForScan(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func spinnerTick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		m.ensureVisible()
		return m, nil

	case spinnerTickMsg:
		if m.phase == phaseScanning {
			m.frame = (m.frame + 1) % len(spinnerFrames)
			return m, spinnerTick()
		}
		return m, nil

	case progressMsg:
		m.done = msg.done
		m.total = msg.total
		m.current = msg.current
		return m, waitForScan(m.scanCh)

	case scanDoneMsg:
		if msg.err != nil {
			m.setNotice("scan failed: "+msg.err.Error(), true)
		}
		m.flagged = scan.Flagged(msg.results)
		m.phase = phaseBrowsing
		m.applyFilter()
		return m, nil

	case detailMsg:
		if sel, ok := m.selected(); ok && sel.AbsPath == msg.abs {
			m.detailText = msg.text
		}
		return m, nil

	case planMsg:
		m.working = false
		if msg.err != nil {
			m.setNotice("cannot resolve: "+msg.err.Error(), true)
			return m, nil
		}
		if msg.plan.Blocked != "" {
			m.setNotice(msg.plan.Repo+": "+msg.plan.Blocked, true)
			return m, nil
		}
		m.confirm = &confirmState{abs: msg.abs, plan: msg.plan}
		return m, nil

	case resolveDoneMsg:
		m.working = false
		if msg.res.Err != nil {
			m.failed[msg.abs] = msg.res.Err.Error()
			m.setNotice("resolve failed: "+msg.res.Err.Error(), true)
		} else {
			m.resolved[msg.abs] = true
			delete(m.failed, msg.abs)
			m.setNotice("resolved: "+msg.res.Message, false)
		}
		return m, m.refreshRepo(msg.abs)

	case refreshMsg:
		m.applyRefresh(msg)
		return m, nil

	case noticeMsg:
		m.setNotice(msg.text, msg.isErr)
		return m, nil

	case execDoneMsg:
		if msg.err != nil {
			m.setNotice(msg.err.Error(), true)
		}
		// The working tree may have changed; refresh the row.
		return m, m.refreshRepo(msg.abs)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.working {
		// Ignore input while an action is running, except hard quit.
		if key == "ctrl+c" {
			m.quit = true
			return m, tea.Quit
		}
		return m, nil
	}
	if m.confirm != nil {
		return m.handleConfirmKey(key)
	}
	if m.showDetail {
		switch key {
		case "esc", "enter", "q":
			m.showDetail = false
			m.detailText = ""
		case "ctrl+c":
			m.quit = true
			return m, tea.Quit
		}
		return m, nil
	}
	if m.filtering {
		return m.handleFilterKey(msg, key)
	}
	return m.handleListKey(key)
}

func (m model) handleListKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c", "q", "esc":
		m.quit = true
		return m, tea.Quit
	case "/":
		m.filtering = true
		m.clearNotice()
		return m, nil
	case "up", "k", "ctrl+p":
		m.move(-1)
	case "down", "j", "ctrl+n":
		m.move(1)
	case "pgup":
		m.move(-m.pageSize())
	case "pgdown":
		m.move(m.pageSize())
	case "home", "g":
		m.cursor = 0
		m.ensureVisible()
	case "end", "G":
		m.cursor = max(0, len(m.filtered)-1)
		m.ensureVisible()
	case "enter":
		return m.openDetail()
	case "s":
		return m.openShell()
	case "e":
		return m.openEditor()
	case "y":
		return m.copyPath()
	case "R":
		return m.startResolve()
	case "r":
		return m.rescan()
	}
	return m, nil
}

func (m model) handleFilterKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.filtering = false
		m.query = ""
		m.applyFilter()
	case "enter":
		m.filtering = false
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "backspace":
		if m.query != "" {
			r := []rune(m.query)
			m.query = string(r[:len(r)-1])
			m.applyFilter()
		}
	case "up":
		m.move(-1)
	case "down":
		m.move(1)
	default:
		if t := msg.Text; t != "" && !strings.ContainsAny(t, "\n\r\t") {
			m.query += t
			m.applyFilter()
		}
	}
	return m, nil
}

func (m model) handleConfirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y", "enter":
		abs := m.confirm.abs
		msg := m.confirm.plan.CommitMsg
		m.confirm = nil
		m.working = true
		m.setNotice("resolving "+abs+" …", false)
		return m, m.runResolve(abs, msg)
	case "n", "N", "esc", "ctrl+c":
		m.confirm = nil
		m.setNotice("resolve cancelled", false)
	}
	return m, nil
}

// --- actions -------------------------------------------------------------

func (m model) openDetail() (tea.Model, tea.Cmd) {
	sel, ok := m.selected()
	if !ok {
		return m, nil
	}
	m.showDetail = true
	m.detailText = ""
	return m, loadDetail(m.ctx, sel)
}

func (m model) openShell() (tea.Model, tea.Cmd) {
	sel, ok := m.selected()
	if !ok {
		return m, nil
	}
	shell := firstNonEmpty(os.Getenv("SHELL"), "/bin/sh")
	cmd := exec.Command(shell)
	cmd.Dir = sel.AbsPath
	abs := sel.AbsPath
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return execDoneMsg{abs: abs, err: err} })
}

func (m model) openEditor() (tea.Model, tea.Cmd) {
	sel, ok := m.selected()
	if !ok {
		return m, nil
	}
	editor := firstNonEmpty(os.Getenv("VISUAL"), os.Getenv("EDITOR"))
	if editor == "" {
		m.setNotice("no $VISUAL or $EDITOR set", true)
		return m, nil
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], ".")...)
	cmd.Dir = sel.AbsPath
	abs := sel.AbsPath
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg { return execDoneMsg{abs: abs, err: err} })
}

func (m model) copyPath() (tea.Model, tea.Cmd) {
	sel, ok := m.selected()
	if !ok {
		return m, nil
	}
	abs := sel.AbsPath
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
		defer cancel()
		if _, err := clipboard.Copy(ctx, []byte(abs)); err != nil {
			return execDoneMsg{abs: abs, err: err}
		}
		return noticeMsg{text: "copied " + abs, isErr: false}
	}
}

type noticeMsg struct {
	text  string
	isErr bool
}

func (m model) startResolve() (tea.Model, tea.Cmd) {
	sel, ok := m.selected()
	if !ok {
		return m, nil
	}
	if m.opts.NoNetwork {
		m.setNotice("resolve needs network; not in --no-network mode", true)
		return m, nil
	}
	if m.resolved[sel.AbsPath] {
		m.setNotice("already resolved", false)
		return m, nil
	}
	m.working = true
	abs := sel.AbsPath
	env := m.opts.Env
	return m, func() tea.Msg {
		plan, err := scan.PlanResolve(m.ctx, abs, "", env)
		return planMsg{abs: abs, plan: plan, err: err}
	}
}

func (m model) runResolve(abs, _ string) tea.Cmd {
	env := m.opts.Env
	timeout := m.opts.Timeout
	return func() tea.Msg {
		res := scan.Resolve(m.ctx, abs, scan.ResolveOptions{Env: env, Timeout: timeout})
		return resolveDoneMsg{abs: abs, res: res}
	}
}

func (m model) refreshRepo(abs string) tea.Cmd {
	opts := scan.Options{Env: m.opts.Env, Timeout: m.opts.Timeout, NoNetwork: m.opts.NoNetwork}
	ctx := m.ctx
	return func() tea.Msg {
		res, ok := scan.InspectRepo(ctx, abs, opts)
		return refreshMsg{abs: abs, res: res, ok: ok}
	}
}

func (m model) rescan() (tea.Model, tea.Cmd) {
	m.phase = phaseScanning
	m.done, m.total, m.current = 0, 0, ""
	m.flagged = nil
	m.filtered = nil
	m.cursor, m.offset = 0, 0
	m.resolved = make(map[string]bool)
	m.failed = make(map[string]string)
	m.scanCh = make(chan tea.Msg, 256)
	m.clearNotice()
	return m, tea.Batch(m.startScan(), spinnerTick())
}

func loadDetail(ctx context.Context, sel scan.Result) tea.Cmd {
	abs := sel.AbsPath
	return func() tea.Msg {
		cmd := exec.CommandContext(ctx, "git", "-C", abs, "-c", "color.status=never", "status")
		out, _ := cmd.CombinedOutput()
		return detailMsg{abs: abs, text: strings.TrimRight(string(out), "\n")}
	}
}

// --- helpers -------------------------------------------------------------

func (m *model) applyRefresh(msg refreshMsg) {
	for i := range m.flagged {
		if m.flagged[i].AbsPath != msg.abs {
			continue
		}
		if msg.ok {
			path := m.flagged[i].Path
			m.flagged[i] = msg.res
			m.flagged[i].Path = path
			if !msg.res.NeedsAttention() {
				m.resolved[msg.abs] = true
			}
		}
		return
	}
}

func (m *model) applyFilter() {
	m.filtered = m.filtered[:0]
	q := strings.TrimSpace(m.query)
	for i, r := range m.flagged {
		if q == "" {
			m.filtered = append(m.filtered, i)
			continue
		}
		if _, ok := fuzzy.Match(q, r.Path); ok {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.ensureVisible()
}

func (m *model) move(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	m.ensureVisible()
}

func (m *model) ensureVisible() {
	size := m.pageSize()
	if size <= 0 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+size {
		m.offset = m.cursor - size + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m model) pageSize() int {
	// Header (2) + footer (2) reserved.
	n := m.height - 4
	if n < 1 {
		return 1
	}
	return n
}

func (m model) selected() (scan.Result, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return scan.Result{}, false
	}
	return m.flagged[m.filtered[m.cursor]], true
}

func (m model) remainingFlagged() int {
	n := 0
	for _, r := range m.flagged {
		if !m.resolved[r.AbsPath] {
			n++
		}
	}
	return n
}

func (m *model) setNotice(text string, isErr bool) {
	m.notice = text
	m.noticeErr = isErr
}

func (m *model) clearNotice() {
	m.notice = ""
	m.noticeErr = false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
