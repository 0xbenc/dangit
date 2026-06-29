package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/0xbenc/dangit/internal/scan"
	"github.com/0xbenc/dangit/internal/termstyle"
	"github.com/0xbenc/termchrome"
	"github.com/0xbenc/termnav/render"
)

func (m model) View() tea.View {
	var body string
	switch {
	case m.phase == phaseScanning:
		body = m.scanningView()
	case m.confirm != nil:
		body = m.confirmView()
	case m.showDetail:
		body = m.detailView()
	default:
		body = m.browsingView()
	}
	view := tea.NewView(body)
	view.AltScreen = !m.opts.NoAltScreen
	return view
}

func (m model) style(role termstyle.Role, text string) string {
	return m.theme.Style(role, text)
}

func (m model) scanningView() string {
	spinner := m.style(termstyle.RoleAccent, m.glyphs.Frame(m.frame))
	title := m.style(termstyle.RoleTitle, "dangit")
	root := m.style(termstyle.RoleMuted, "scanning "+m.opts.RootLabel)

	progress := "discovering repositories…"
	if m.total > 0 {
		progress = fmt.Sprintf("%d/%d", m.done, m.total)
	}
	line := fmt.Sprintf("%s  %s  %s", spinner, m.style(termstyle.RolePrimary, progress), root)

	lines := []string{"", "  " + title, "", "  " + line}
	if m.current != "" {
		lines = append(lines, "  "+m.style(termstyle.RoleMuted, truncate(m.current, m.width-4)))
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m model) browsingView() string {
	var b strings.Builder

	remaining := m.remainingFlagged()
	header := m.style(termstyle.RoleTitle, fmt.Sprintf("Needs attention (%d)", remaining))
	root := m.style(termstyle.RoleMuted, m.opts.RootLabel)
	b.WriteString(header + "   " + root + "\n")

	if m.filtering || m.query != "" {
		cursor := ""
		if m.filtering {
			cursor = m.style(termstyle.RoleAccent, "▏")
		}
		b.WriteString(m.style(termstyle.RoleMuted, "filter: ") + m.query + cursor + "\n")
	} else {
		b.WriteString("\n")
	}

	if len(m.filtered) == 0 {
		empty := "No repositories need attention. 🎉"
		if m.query != "" {
			empty = "No matches for " + fmt.Sprintf("%q", m.query) + "."
		}
		b.WriteString("\n  " + m.style(termstyle.RoleSuccess, empty) + "\n")
	} else {
		b.WriteString(m.listView())
	}

	b.WriteString(m.footer())
	return b.String()
}

func (m model) listView() string {
	size := m.pageSize()
	end := m.offset + size
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	// Align branch column to the widest visible path in the window.
	pathWidth := 0
	for i := m.offset; i < end; i++ {
		r := m.flagged[m.filtered[i]]
		if w := termstyle.VisibleWidth(r.Path); w > pathWidth {
			pathWidth = w
		}
	}

	var b strings.Builder
	for i := m.offset; i < end; i++ {
		r := m.flagged[m.filtered[i]]
		selected := i == m.cursor

		marker := m.style(termstyle.RoleAccent, "•")
		if m.resolved[r.AbsPath] {
			marker = m.style(termstyle.RoleSuccess, "✓")
		} else if reason, bad := m.failed[r.AbsPath]; bad {
			_ = reason
			marker = m.style(termstyle.RoleDanger, "✗")
		}

		base := func(s string) string { return s }
		if selected {
			base = func(s string) string { return m.style(termstyle.RoleSelected, s) }
		}
		hl := func(s string) string { return m.style(termstyle.RoleSearch, s) }
		path := render.HighlightMatches(r.Path, m.filtPos[i], pathWidth, base, hl)
		if pad := pathWidth - termstyle.VisibleWidth(r.Path); pad > 0 {
			path += base(strings.Repeat(" ", pad))
		}
		branch := m.style(termstyle.RolePrimary, "("+r.Branch+")")

		var status string
		if m.resolved[r.AbsPath] {
			status = m.style(termstyle.RoleMuted, "resolved")
		} else {
			status = statusParts(m.theme, r)
		}

		prefix := "  "
		if selected {
			prefix = m.style(termstyle.RoleAccent, "▸ ")
		}
		fmt.Fprintf(&b, "%s%s %s  %s  %s\n", prefix, marker, path, branch, status)
	}

	// Pad the list region to a stable height.
	for i := end - m.offset; i < size; i++ {
		b.WriteString("\n")
	}
	return b.String()
}

func (m model) detailView() string {
	sel, ok := m.selected()
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.style(termstyle.RoleTitle, sel.Path) + "  " +
		m.style(termstyle.RolePrimary, "("+sel.Branch+")") + "\n")
	b.WriteString(m.style(termstyle.RoleMuted, sel.AbsPath) + "\n\n")
	b.WriteString("  " + statusParts(m.theme, sel) + "\n\n")
	if m.detailText == "" {
		b.WriteString(m.style(termstyle.RoleMuted, "  loading git status…") + "\n")
	} else {
		for _, line := range strings.Split(m.detailText, "\n") {
			b.WriteString("  " + truncate(line, m.width-2) + "\n")
		}
	}
	b.WriteString("\n" + m.style(termstyle.RoleMuted, "  enter/esc back") + "\n")
	return b.String()
}

func (m model) confirmView() string {
	plan := m.confirm.plan
	var b strings.Builder
	b.WriteString("\n  " + m.style(termstyle.RoleTitle, "Resolve "+plan.Repo+"?") + "\n\n")
	if plan.WillCommit {
		b.WriteString("  " + m.style(termstyle.RoleWarning, "commit") + "  " +
			m.style(termstyle.RoleMuted, fmt.Sprintf("%q (%d file(s))", plan.CommitMsg, len(plan.Files))) + "\n")
	}
	if plan.WillPull {
		b.WriteString("  " + m.style(termstyle.RoleInfo, "pull") + "    " +
			m.style(termstyle.RoleMuted, "--rebase from upstream") + "\n")
	}
	if plan.WillPush {
		b.WriteString("  " + m.style(termstyle.RoleSuccess, "push") + "    " +
			m.style(termstyle.RoleMuted, "to "+plan.Branch+"'s upstream") + "\n")
	}
	b.WriteString("\n  " + m.style(termstyle.RoleAccent, "y") + " confirm   " +
		m.style(termstyle.RoleAccent, "n") + " cancel\n")
	return b.String()
}

func (m model) footer() string {
	var hints []termchrome.KeyHint
	if m.filtering {
		hints = []termchrome.KeyHint{
			{Key: "type", Label: "to filter"},
			{Key: "enter", Label: "apply"},
			{Key: "esc", Label: "clear"},
		}
	} else {
		hints = []termchrome.KeyHint{
			{Key: "↑↓", Label: "move"},
			{Key: "/", Label: "filter"},
			{Key: "⏎", Label: "detail"},
			{Key: "s", Label: "shell"},
			{Key: "e", Label: "editor"},
			{Key: "y", Label: "copy"},
			{Key: "R", Label: "resolve"},
			{Key: "r", Label: "rescan"},
			{Key: "q", Label: "quit"},
		}
	}
	footer := "\n" + m.style(termstyle.RoleMuted, termchrome.Footer(hints, m.width))
	if m.notice != "" {
		role := termstyle.RoleInfo
		if m.noticeErr {
			role = termstyle.RoleDanger
		}
		footer += "\n" + m.style(role, truncate(m.notice, m.width))
	} else {
		footer += "\n"
	}
	return footer
}

// statusParts renders the colored status descriptors for a flagged repo.
func statusParts(theme termstyle.Theme, r scan.Result) string {
	var parts []string
	if r.HasChanges {
		parts = append(parts, theme.Style(termstyle.RoleWarning, "changes"))
	}
	if r.Ahead != "" && r.Ahead != scan.StateNone {
		q := r.Ahead
		if r.Ahead == scan.AheadNoUpstream {
			q = "no remote"
		}
		parts = append(parts, theme.Style(termstyle.RoleInfo, "ahead")+" "+
			theme.Style(termstyle.RoleMuted, "("+q+")"))
	}
	if r.Behind != "" && r.Behind != scan.StateNone {
		parts = append(parts, theme.Style(termstyle.RoleDanger, "behind")+" "+
			theme.Style(termstyle.RoleMuted, "("+r.Behind+")"))
	}
	if len(parts) == 0 {
		return theme.Style(termstyle.RoleMuted, "ok")
	}
	return strings.Join(parts, ", ")
}

// truncate shortens s to fit width visible columns, adding an ellipsis.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	return termstyle.TruncateWith(s, width, "…")
}
