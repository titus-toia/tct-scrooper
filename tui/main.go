package main

import (
	"fmt"
	"os"
	"time"

	"tui/db"
	"tui/styles"
	"tui/views"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
)

type tab int

const (
	tabDashboard tab = iota
	tabData
	tabLogs
)

type model struct {
	db            *db.Client
	activeTab     tab
	width, height int
	notification  string
	notifyUntil   time.Time

	dashboard views.Dashboard
	data      views.Data
	logs      views.Logs
}

type tickMsg time.Time
type logTickMsg time.Time
type refreshMsg struct{}

func initialModel(dbClient *db.Client, logPath string) model {
	return model{
		db:        dbClient,
		activeTab: tabDashboard,
		dashboard: views.NewDashboard(dbClient, logPath),
		data:      views.NewData(dbClient),
		logs:      views.NewLogs(dbClient),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.dashboard.Init(),
		m.data.Init(),
		m.logs.Init(),
		tickCmd(),
		logTickCmd(),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func logTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return logTickMsg(t)
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "d":
			m.activeTab = tabDashboard
		case "p":
			m.activeTab = tabData
		case "l":
			m.activeTab = tabLogs
		case "tab":
			m.activeTab = (m.activeTab + 1) % 3
		case "r":
			m.notification = "Refreshed"
			m.notifyUntil = time.Now().Add(2 * time.Second)
			return m, m.refreshActive()
		case "s":
			if err := m.db.ScrapeNow(); err == nil {
				m.notification = "Scrape command sent!"
				m.notifyUntil = time.Now().Add(2 * time.Second)
			}
		case "m":
			if err := m.db.RunMedia(); err == nil {
				m.notification = "Media worker triggered!"
				m.notifyUntil = time.Now().Add(2 * time.Second)
			}
		case "e":
			if err := m.db.RunEnrichment(); err == nil {
				m.notification = "Enrichment worker triggered!"
				m.notifyUntil = time.Now().Add(2 * time.Second)
			}
		case "h":
			if err := m.db.RunHealthcheck(); err == nil {
				m.notification = "Healthcheck worker triggered!"
				m.notifyUntil = time.Now().Add(2 * time.Second)
			}
		case "c":
			if url := m.data.GetSelectedURL(); url != "" {
				m.notification = "URL copied!"
				m.notifyUntil = time.Now().Add(2 * time.Second)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Propagate size to all views
		m.dashboard = m.dashboard.SetSize(msg.Width, msg.Height-4)
		m.data = m.data.SetSize(msg.Width, msg.Height-4)
		m.logs = m.logs.SetSize(msg.Width, msg.Height-4)

	case tickMsg:
		cmds = append(cmds, m.refreshActive(), tickCmd())

	case logTickMsg:
		cmds = append(cmds, m.dashboard.RefreshLog(), logTickCmd())

	case refreshMsg:
		cmds = append(cmds, m.refreshActive())
	}

	// Always route data messages to all views (for initial load)
	// Route key messages only to active tab
	switch msg.(type) {
	case tea.KeyMsg:
		switch m.activeTab {
		case tabDashboard:
			newDashboard, cmd := m.dashboard.Update(msg)
			m.dashboard = newDashboard.(views.Dashboard)
			cmds = append(cmds, cmd)
		case tabData:
			newData, cmd := m.data.Update(msg)
			m.data = newData.(views.Data)
			cmds = append(cmds, cmd)
		case tabLogs:
			newLogs, cmd := m.logs.Update(msg)
			m.logs = newLogs.(views.Logs)
			cmds = append(cmds, cmd)
		}
	default:
		// Route other messages to all views
		newDashboard, cmd1 := m.dashboard.Update(msg)
		m.dashboard = newDashboard.(views.Dashboard)
		cmds = append(cmds, cmd1)

		newData, cmd2 := m.data.Update(msg)
		m.data = newData.(views.Data)
		cmds = append(cmds, cmd2)

		newLogs, cmd3 := m.logs.Update(msg)
		m.logs = newLogs.(views.Logs)
		cmds = append(cmds, cmd3)
	}

	return m, tea.Batch(cmds...)
}

func (m model) refreshActive() tea.Cmd {
	switch m.activeTab {
	case tabDashboard:
		return m.dashboard.Refresh()
	case tabData:
		return m.data.Refresh()
	case tabLogs:
		return m.logs.Refresh()
	}
	return nil
}

func (m model) View() string {
	tabs := m.renderTabs()
	content := m.renderContent()
	statusBar := m.renderStatusBar()

	return lipgloss.JoinVertical(lipgloss.Left, tabs, content, statusBar)
}

func (m model) renderTabs() string {
	tabNames := []string{"Dashboard", "Data", "Logs"}
	var rendered []string
	for i, name := range tabNames {
		if tab(i) == m.activeTab {
			rendered = append(rendered, styles.TabActive.Render(name))
		} else {
			rendered = append(rendered, styles.TabInactive.Render(name))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...) + "\n"
}

func (m model) renderContent() string {
	switch m.activeTab {
	case tabDashboard:
		return m.dashboard.View()
	case tabData:
		return m.data.View()
	case tabLogs:
		return m.logs.View()
	}
	return ""
}

func (m model) renderStatusBar() string {
	left := "d Dash  p Data  l Log  r Refresh  s Scrape  m Media  e Enrich  h Health  q Quit"
	right := ""
	if time.Now().Before(m.notifyUntil) {
		right = styles.Notification.Render(m.notification)
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 0 {
		gap = 0
	}

	return styles.StatusBar.Render(left) + lipgloss.NewStyle().Width(gap).Render("") + right
}

func main() {
	_ = godotenv.Load() // Load .env if present

	postgresURL := os.Getenv("SUPABASE_DB_URL")
	if postgresURL == "" {
		fmt.Fprintf(os.Stderr, "Error: SUPABASE_DB_URL environment variable is required\n")
		os.Exit(1)
	}

	sqlitePath := os.Getenv("DB_PATH")
	if sqlitePath == "" {
		sqlitePath = "scraper.db"
	}

	logPath := os.Getenv("LOG_PATH")
	if logPath == "" {
		logPath = "daemon.log"
	}

	dbClient, err := db.New(postgresURL, sqlitePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to database: %v\n", err)
		os.Exit(1)
	}
	defer dbClient.Close()

	p := tea.NewProgram(
		initialModel(dbClient, logPath),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
