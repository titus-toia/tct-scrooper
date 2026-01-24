package views

import (
	"fmt"
	"strings"

	"tui-go/db"
	"tui-go/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var logLevels = []string{"ALL", "DEBUG", "INFO", "WARN", "ERROR"}

type logsMsg struct {
	logs []db.ScrapeLog
}

type Logs struct {
	db            *db.Client
	width, height int
	logs          []db.ScrapeLog
	levelIndex    int
	scrollOffset  int
}

func NewLogs(dbClient *db.Client) Logs {
	return Logs{db: dbClient}
}

func (l Logs) Init() tea.Cmd {
	return l.Refresh()
}

func (l Logs) Refresh() tea.Cmd {
	return func() tea.Msg {
		level := logLevels[l.levelIndex]
		var levelPtr *string
		if level != "ALL" {
			levelPtr = &level
		}
		logs, _ := l.db.GetRecentLogs(200, levelPtr)
		return logsMsg{logs}
	}
}

func (l Logs) SetSize(w, h int) {
	l.width = w
	l.height = h
}

func (l Logs) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case logsMsg:
		l.logs = msg.logs
		l.scrollOffset = 0

	case tea.WindowSizeMsg:
		l.width = msg.Width
		l.height = msg.Height - 4

	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if l.levelIndex > 0 {
				l.levelIndex--
				return l, l.Refresh()
			}
		case "right", "l":
			if l.levelIndex < len(logLevels)-1 {
				l.levelIndex++
				return l, l.Refresh()
			}
		case "up", "k":
			if l.scrollOffset > 0 {
				l.scrollOffset--
			}
		case "down", "j":
			maxScroll := len(l.logs) - l.visibleLines()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if l.scrollOffset < maxScroll {
				l.scrollOffset++
			}
		case "g":
			l.scrollOffset = 0
		case "G":
			maxScroll := len(l.logs) - l.visibleLines()
			if maxScroll < 0 {
				maxScroll = 0
			}
			l.scrollOffset = maxScroll
		}
	}
	return l, nil
}

func (l Logs) visibleLines() int {
	return l.height - 6
}

func (l Logs) View() string {
	filter := l.renderFilter()
	logOutput := l.renderLogs()

	return lipgloss.JoinVertical(lipgloss.Left,
		styles.Title.Render("Logs"),
		filter,
		"",
		logOutput,
	)
}

func (l Logs) renderFilter() string {
	var parts []string
	for i, level := range logLevels {
		if i == l.levelIndex {
			parts = append(parts, styles.TabActive.Render("["+level+"]"))
		} else {
			parts = append(parts, styles.TabInactive.Render(level))
		}
	}
	return "Filter: " + strings.Join(parts, " ") + "  (←/→ to change)"
}

func (l Logs) renderLogs() string {
	if len(l.logs) == 0 {
		return styles.Muted.Render("No logs")
	}

	visible := l.visibleLines()
	if visible < 1 {
		visible = 10
	}

	start := l.scrollOffset
	end := start + visible
	if end > len(l.logs) {
		end = len(l.logs)
	}

	var lines []string
	for i := start; i < end; i++ {
		log := l.logs[i]
		lines = append(lines, l.formatLog(log))
	}

	scrollInfo := fmt.Sprintf("  [%d-%d of %d]", start+1, end, len(l.logs))
	header := styles.Muted.Render(scrollInfo)

	return header + "\n" + strings.Join(lines, "\n")
}

func (l Logs) formatLog(log db.ScrapeLog) string {
	ts := log.Timestamp.Format("15:04:05")
	level := fmt.Sprintf("%-5s", log.Level)

	var levelStyle lipgloss.Style
	switch log.Level {
	case "DEBUG":
		levelStyle = styles.Muted
	case "INFO":
		levelStyle = styles.StatusSuccess
	case "WARN":
		levelStyle = styles.StatusPending
	case "ERROR", "FATAL":
		levelStyle = styles.StatusError
	default:
		levelStyle = lipgloss.NewStyle()
	}

	site := ""
	if log.SiteID != nil {
		site = fmt.Sprintf("[%s] ", *log.SiteID)
	}

	msg := log.Message
	maxLen := l.width - 25
	if maxLen > 0 && len(msg) > maxLen {
		msg = msg[:maxLen-3] + "..."
	}

	return fmt.Sprintf("%s %s %s%s",
		styles.Muted.Render(ts),
		levelStyle.Render(level),
		styles.Muted.Render(site),
		msg,
	)
}
