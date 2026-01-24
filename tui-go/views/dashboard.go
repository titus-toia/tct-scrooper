package views

import (
	"fmt"
	"time"

	"tui-go/db"
	"tui-go/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dashboardDataMsg struct {
	stats       []db.SiteStats
	runs        []db.ScrapeRun
	propCount   int
	snapCount   int
	unsyncCount int
}

type Dashboard struct {
	db            *db.Client
	width, height int
	stats         []db.SiteStats
	runs          []db.ScrapeRun
	propCount     int
	snapCount     int
	unsyncCount   int
}

func NewDashboard(dbClient *db.Client) Dashboard {
	return Dashboard{db: dbClient}
}

func (d Dashboard) Init() tea.Cmd {
	return d.Refresh()
}

func (d Dashboard) Refresh() tea.Cmd {
	return func() tea.Msg {
		stats, _ := d.db.GetSiteStats()
		runs, _ := d.db.GetRecentRuns(10)
		propCount, _ := d.db.GetPropertyCount()
		snapCount, _ := d.db.GetSnapshotCount()
		unsyncCount, _ := d.db.GetUnsyncedCount()
		return dashboardDataMsg{stats, runs, propCount, snapCount, unsyncCount}
	}
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
		d.propCount = msg.propCount
		d.snapCount = msg.snapCount
		d.unsyncCount = msg.unsyncCount
	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height - 4
	}
	return d, nil
}

func (d Dashboard) View() string {
	statCards := d.renderStatCards()
	siteCards := d.renderSiteCards()
	runsTable := d.renderRunsTable()

	return lipgloss.JoinVertical(lipgloss.Left,
		styles.Title.Render("Dashboard"),
		statCards,
		"",
		siteCards,
		"",
		styles.Title.Render("Recent Runs"),
		runsTable,
	)
}

func (d Dashboard) renderStatCards() string {
	cards := []string{
		d.renderStatCard("Properties", fmt.Sprintf("%d", d.propCount)),
		d.renderStatCard("Snapshots", fmt.Sprintf("%d", d.snapCount)),
		d.renderStatCard("Unsynced", fmt.Sprintf("%d", d.unsyncCount)),
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

	resumePage := "—"
	if s.ResumeFromPage > 0 {
		resumePage = fmt.Sprintf("%d/∞", s.ResumeFromPage)
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		styles.StatValue.Render(s.SiteID),
		statusStyle.Render(status),
		styles.StatLabel.Render(fmt.Sprintf("Last: %s", lastRun)),
		styles.StatLabel.Render(fmt.Sprintf("Page: %s", resumePage)),
		styles.StatLabel.Render(fmt.Sprintf("Props: %d", s.TotalProperties)),
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
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
