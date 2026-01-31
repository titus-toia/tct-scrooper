package views

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"tui/db"
	"tui/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dashboardDataMsg struct {
	stats         []db.SiteStats
	runs          []db.ScrapeRun
	cityStats     []db.CityStats
	propCount     int
	listingCount  int
	activeCount   int
	mediaQueue    int
}

type logTailMsg struct {
	lines        []string
	modTime      time.Time
	daemonActive bool
}

type Dashboard struct {
	db            *db.Client
	width, height int
	stats         []db.SiteStats
	runs          []db.ScrapeRun
	cityStats     []db.CityStats
	propCount     int
	listingCount  int
	activeCount   int
	mediaQueue    int
	logLines      []string
	logPath       string
	logScroll     int       // scroll offset (0 = bottom/newest)
	logViewport   int       // visible lines
	logBuffer     int       // total lines to keep
	logModTime    time.Time // last modification time of log file
	daemonActive  bool      // whether systemd service is active
}

func NewDashboard(dbClient *db.Client, logPath string) Dashboard {
	if logPath == "" {
		logPath = "daemon.log"
	}
	return Dashboard{
		db:          dbClient,
		logPath:     logPath,
		logViewport: 30,
		logBuffer:   200,
	}
}

func (d Dashboard) Init() tea.Cmd {
	return tea.Batch(d.Refresh(), d.tailLog())
}

func (d Dashboard) Refresh() tea.Cmd {
	return func() tea.Msg {
		stats, _ := d.db.GetSiteStats()
		runs, _ := d.db.GetRecentRuns(10)
		cityStats, _ := d.db.GetCityStats()
		propCount, _ := d.db.GetPropertyCount()
		listingCount, _ := d.db.GetListingCount()
		activeCount, _ := d.db.GetActiveListingCount()
		mediaQueue, _ := d.db.GetPendingMediaCount()
		return dashboardDataMsg{stats, runs, cityStats, propCount, listingCount, activeCount, mediaQueue}
	}
}

func (d Dashboard) tailLog() tea.Cmd {
	return func() tea.Msg {
		lines, modTime := readLastLines(d.logPath, d.logBuffer)
		active := isDaemonActive()
		return logTailMsg{lines, modTime, active}
	}
}

func isDaemonActive() bool {
	out, err := exec.Command("systemctl", "is-active", "tct_scrooper").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}

func readLastLines(path string, n int) ([]string, time.Time) {
	info, err := os.Stat(path)
	if err != nil {
		return []string{"(no log file)"}, time.Time{}
	}
	modTime := info.ModTime()

	f, err := os.Open(path)
	if err != nil {
		return []string{"(no log file)"}, time.Time{}
	}
	defer f.Close()

	var allLines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if len(allLines) == 0 {
		return []string{"(empty log)"}, modTime
	}

	start := len(allLines) - n
	if start < 0 {
		start = 0
	}
	return allLines[start:], modTime
}

func (d Dashboard) SetSize(w, h int) {
	d.width = w
	d.height = h
}

func (d Dashboard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dashboardDataMsg:
		d.stats = msg.stats
		d.runs = msg.runs
		d.cityStats = msg.cityStats
		d.propCount = msg.propCount
		d.listingCount = msg.listingCount
		d.activeCount = msg.activeCount
		d.mediaQueue = msg.mediaQueue
		return d, d.tailLog()
	case logTailMsg:
		d.logLines = msg.lines
		d.logModTime = msg.modTime
		d.daemonActive = msg.daemonActive
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height - 4
	case tea.KeyMsg:
		maxScroll := len(d.logLines) - d.logViewport
		if maxScroll < 0 {
			maxScroll = 0
		}
		switch msg.String() {
		case "up", "k":
			d.logScroll++
			if d.logScroll > maxScroll {
				d.logScroll = maxScroll
			}
		case "down", "j":
			d.logScroll--
			if d.logScroll < 0 {
				d.logScroll = 0
			}
		case "pgup":
			d.logScroll += 10
			if d.logScroll > maxScroll {
				d.logScroll = maxScroll
			}
		case "pgdown":
			d.logScroll -= 10
			if d.logScroll < 0 {
				d.logScroll = 0
			}
		case "home":
			d.logScroll = maxScroll
		case "end":
			d.logScroll = 0
		}
	}
	return d, nil
}

func (d Dashboard) View() string {
	statCards := d.renderStatCards()
	siteCards := d.renderSiteCards()
	cityCards := d.renderCityCards()
	runsTable := d.renderRunsTable()
	logTail := d.renderLogTail()

	return lipgloss.JoinVertical(lipgloss.Left,
		styles.Title.Render("Dashboard"),
		statCards,
		"",
		siteCards,
		"",
		cityCards,
		"",
		styles.Title.Render("Recent Runs"),
		runsTable,
		"",
		logTail,
	)
}

func (d Dashboard) renderLogTail() string {
	if len(d.logLines) == 0 {
		content := styles.Muted.Render("(waiting for logs...)")
		return styles.LogBox.Width(d.width - 4).Render(content)
	}

	// Calculate visible window (from end, with scroll offset)
	total := len(d.logLines)
	endIdx := total - d.logScroll
	startIdx := endIdx - d.logViewport
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > total {
		endIdx = total
	}

	visibleLines := d.logLines[startIdx:endIdx]
	maxLineWidth := d.width - 8

	var lines []string
	for _, line := range visibleLines {
		styled := d.styleLogLine(line, maxLineWidth)
		lines = append(lines, styled)
	}

	content := strings.Join(lines, "\n")

	// Status indicator based on daemon status and scroll position
	scrollInfo := ""
	if !d.daemonActive {
		scrollInfo = styles.StatusError.Render(" ● STOPPED ")
	} else if d.logScroll > 0 {
		scrollInfo = styles.StatusPending.Render(fmt.Sprintf(" ↑%d ", d.logScroll))
	} else {
		scrollInfo = styles.StatusSuccess.Render(" ● LIVE ")
	}

	header := styles.Title.Render("Live Log") + scrollInfo +
		styles.Muted.Render(fmt.Sprintf("[%d-%d/%d]", startIdx+1, endIdx, total))

	boxContent := header + "\n" + content
	return styles.LogBox.Width(d.width - 4).Render(boxContent)
}

func (d Dashboard) styleLogLine(line string, maxWidth int) string {
	line = truncate(line, maxWidth)

	// Parse timestamp if present (format: 2024/01/25 10:30:45)
	if len(line) > 19 && (line[4] == '/' || line[10] == ' ') {
		timestamp := line[:19]
		rest := line[19:]

		styledTs := styles.LogTimestamp.Render(timestamp)

		if strings.Contains(rest, "ERROR") || strings.Contains(rest, "error") {
			return styledTs + styles.StatusError.Render(rest)
		} else if strings.Contains(rest, "WARN") || strings.Contains(rest, "warn") {
			return styledTs + styles.StatusPending.Render(rest)
		} else if strings.Contains(rest, "INFO") || strings.Contains(rest, "info") {
			return styledTs + styles.LogInfo.Render(rest)
		}
		return styledTs + rest
	}

	// No timestamp - style whole line
	if strings.Contains(line, "ERROR") || strings.Contains(line, "error") {
		return styles.StatusError.Render(line)
	} else if strings.Contains(line, "WARN") || strings.Contains(line, "warn") {
		return styles.StatusPending.Render(line)
	} else if strings.Contains(line, "INFO") || strings.Contains(line, "info") {
		return styles.LogInfo.Render(line)
	}
	return line
}

func (d Dashboard) renderStatCards() string {
	cards := []string{
		d.renderStatCard("Properties", fmt.Sprintf("%d", d.propCount)),
		d.renderStatCard("Listings", fmt.Sprintf("%d", d.listingCount)),
		d.renderStatCard("Active", fmt.Sprintf("%d", d.activeCount)),
		d.renderStatCard("Media Q", fmt.Sprintf("%d", d.mediaQueue)),
		d.renderStatCard("Sites", fmt.Sprintf("%d", len(d.stats))),
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
}

func (d Dashboard) renderStatCard(label, value string) string {
	content := lipgloss.JoinVertical(lipgloss.Center,
		styles.StatValue.Render(value),
		styles.StatLabel.Render(label),
	)
	return styles.CardBorder.Width(16).Render(content)
}

func (d Dashboard) renderSiteCards() string {
	if len(d.stats) == 0 {
		return styles.Muted.Render("No sites configured")
	}

	var cards []string
	for _, s := range d.stats {
		cards = append(cards, d.renderSiteCard(s))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
}

func (d Dashboard) renderCityCards() string {
	if len(d.cityStats) == 0 {
		return ""
	}

	var cards []string
	for _, c := range d.cityStats {
		cards = append(cards, d.renderCityCard(c))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cards...)
}

func (d Dashboard) renderCityCard(c db.CityStats) string {
	avgPrice := ""
	if c.AvgPrice > 0 {
		avgPrice = fmt.Sprintf("$%dk", c.AvgPrice/1000)
	} else {
		avgPrice = "-"
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		styles.StatValue.Render(fmt.Sprintf("%s, %s", c.City, c.Province)),
		styles.StatLabel.Render(fmt.Sprintf("Props: %d", c.PropertyCount)),
		styles.StatLabel.Render(fmt.Sprintf("Active: %d", c.ActiveCount)),
		styles.StatLabel.Render(fmt.Sprintf("Avg: %s", avgPrice)),
	)
	return styles.CityCardBorder.Width(20).Render(content)
}

func (d Dashboard) renderSiteCard(s db.SiteStats) string {
	status := "○ never run"
	statusStyle := styles.StatusPending
	if s.LastRunStatus != nil {
		switch *s.LastRunStatus {
		case "completed":
			status = "✓ completed"
			statusStyle = styles.StatusSuccess
		case "failed":
			status = "✗ failed"
			statusStyle = styles.StatusError
		case "running":
			status = "◐ running"
			statusStyle = styles.StatusPending
		}
	}

	lastRun := "never"
	if s.LastRunAt != nil {
		lastRun = relativeTime(*s.LastRunAt)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		styles.StatValue.Render(s.SiteID),
		statusStyle.Render(status),
		styles.StatLabel.Render(fmt.Sprintf("Last: %s", lastRun)),
		styles.StatLabel.Render(fmt.Sprintf("Props: %d", s.TotalProperties)),
		styles.StatLabel.Render(fmt.Sprintf("Listings: %d", s.TotalListings)),
		styles.StatLabel.Render(fmt.Sprintf("Rate: %.0f%%", s.SuccessRate*100)),
	)
	return styles.SiteCardBorder.Width(24).Render(content)
}

func (d Dashboard) renderRunsTable() string {
	if len(d.runs) == 0 {
		return styles.Muted.Render("No runs yet")
	}

	header := fmt.Sprintf("%-12s %-10s %-10s %6s %6s %6s %6s",
		"Site", "Status", "Started", "Found", "New", "Relist", "Errors")
	rows := styles.TableHeader.Render(header) + "\n"

	for _, r := range d.runs {
		status := r.Status
		statusStyle := styles.StatusPending
		switch r.Status {
		case "completed":
			statusStyle = styles.StatusSuccess
		case "failed":
			statusStyle = styles.StatusError
		}

		started := r.StartedAt.Format("15:04:05")
		row := fmt.Sprintf("%-12s %s %-10s %6d %6d %6d %6d",
			truncate(r.SiteID, 12),
			statusStyle.Render(fmt.Sprintf("%-10s", status)),
			started,
			r.ListingsFound,
			r.PropertiesNew,
			r.PropertiesRelisted,
			r.ErrorsCount,
		)
		rows += row + "\n"
	}
	return rows
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return s[:max-1] + "…"
}
