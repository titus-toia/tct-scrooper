package views

import (
	"fmt"
	"strings"

	"tui-go/db"
	"tui-go/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type dataMsg struct {
	properties []db.Property
	total      int
}

type snapshotsMsg struct {
	snapshots []db.Snapshot
}

type Data struct {
	db             *db.Client
	width, height  int
	properties     []db.Property
	snapshots      []db.Snapshot
	selectedRow    int
	unsyncedOnly   bool
	selectedPropID string
	dbPage         int // current database page (0-indexed)
	dbPageSize     int // items per database page
	totalProps     int // total properties in DB
}

func NewData(dbClient *db.Client) Data {
	return Data{db: dbClient, dbPageSize: 100}
}

func (d Data) Init() tea.Cmd {
	return d.Refresh()
}

func (d Data) Refresh() tea.Cmd {
	return func() tea.Msg {
		props, _ := d.db.GetProperties(d.dbPageSize, d.dbPage*d.dbPageSize, d.unsyncedOnly)
		total, _ := d.db.GetPropertyCount()
		return dataMsg{props, total}
	}
}

func (d Data) SetSize(w, h int) {
	d.width = w
	d.height = h
}

func (d Data) GetSelectedURL() string {
	if len(d.snapshots) > 0 {
		return d.snapshots[0].URL
	}
	return ""
}

func (d Data) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dataMsg:
		d.properties = msg.properties
		d.totalProps = msg.total
		if d.selectedRow >= len(d.properties) {
			d.selectedRow = 0
		}
		if len(d.properties) > 0 {
			return d, d.loadSnapshots(d.properties[d.selectedRow].ID)
		}

	case snapshotsMsg:
		d.snapshots = msg.snapshots

	case tea.WindowSizeMsg:
		d.width = msg.Width
		d.height = msg.Height - 4

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if d.selectedRow > 0 {
				d.selectedRow--
				if len(d.properties) > 0 {
					return d, d.loadSnapshots(d.properties[d.selectedRow].ID)
				}
			}
		case "down", "j":
			if len(d.properties) > 0 && d.selectedRow < len(d.properties)-1 {
				d.selectedRow++
				return d, d.loadSnapshots(d.properties[d.selectedRow].ID)
			}
		case "pgdown", "ctrl+d":
			if len(d.properties) > 0 {
				d.selectedRow += 10
				if d.selectedRow >= len(d.properties) {
					d.selectedRow = len(d.properties) - 1
				}
				return d, d.loadSnapshots(d.properties[d.selectedRow].ID)
			}
		case "pgup", "ctrl+u":
			if len(d.properties) > 0 {
				d.selectedRow -= 10
				if d.selectedRow < 0 {
					d.selectedRow = 0
				}
				return d, d.loadSnapshots(d.properties[d.selectedRow].ID)
			}
		case "home", "g":
			if len(d.properties) > 0 {
				d.selectedRow = 0
				return d, d.loadSnapshots(d.properties[d.selectedRow].ID)
			}
		case "end", "G":
			if len(d.properties) > 0 {
				d.selectedRow = len(d.properties) - 1
				return d, d.loadSnapshots(d.properties[d.selectedRow].ID)
			}
		case "u":
			d.unsyncedOnly = !d.unsyncedOnly
			d.selectedRow = 0
			return d, d.Refresh()
		case "1", "2", "3", "4", "5", "6", "7", "8", "9", "0":
			// Jump to database page (1=page 1, 0=page 10)
			pageNum := int(msg.String()[0] - '0')
			if pageNum == 0 {
				pageNum = 10
			}
			totalPages := d.getTotalDBPages()
			if pageNum <= totalPages {
				d.dbPage = pageNum - 1
				d.selectedRow = 0
				return d, d.Refresh()
			}
		case "[":
			// Previous database page
			if d.dbPage > 0 {
				d.dbPage--
				d.selectedRow = 0
				return d, d.Refresh()
			}
		case "]":
			// Next database page
			if d.dbPage < d.getTotalDBPages()-1 {
				d.dbPage++
				d.selectedRow = 0
				return d, d.Refresh()
			}
		}
	}
	return d, nil
}

func (d Data) loadSnapshots(propID string) tea.Cmd {
	d.selectedPropID = propID
	return func() tea.Msg {
		snaps, _ := d.db.GetSnapshotsForProperty(propID)
		return snapshotsMsg{snaps}
	}
}

func (d Data) getVisibleRows() int {
	rows := 25
	if d.height > 0 {
		rows = (d.height * 60) / 100
		if rows < 10 {
			rows = 10
		}
	}
	return rows
}

func (d Data) getTotalDBPages() int {
	if d.dbPageSize == 0 || d.totalProps == 0 {
		return 1
	}
	return (d.totalProps + d.dbPageSize - 1) / d.dbPageSize
}

func (d Data) View() string {
	filterStatus := "All"
	if d.unsyncedOnly {
		filterStatus = "Unsynced only"
	}

	// Position counter - show global position across all pages
	globalPos := d.dbPage*d.dbPageSize + d.selectedRow + 1
	position := fmt.Sprintf("  %d/%d", globalPos, d.totalProps)
	pageInfo := fmt.Sprintf("  Page %d/%d", d.dbPage+1, d.getTotalDBPages())

	propsTable := d.renderPropertiesTable()
	bottomPanel := d.renderBottomPanel()

	header := styles.Title.Render("Properties") +
		styles.StatValue.Render(position) +
		styles.StatLabel.Render(pageInfo) +
		"  " + styles.Muted.Render(fmt.Sprintf("[u] Filter: %s  [1-0] Page  [[ ]] Prev/Next", filterStatus))

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		propsTable,
		"",
		bottomPanel,
	)
}

func (d Data) renderPropertiesTable() string {
	header := fmt.Sprintf("%-35s %-12s %10s %4s %4s %7s %-8s %5s %6s",
		"Address", "City", "Price", "Bed", "Bath", "SqFt", "Type", "List", "Sync")
	rows := styles.TableHeader.Render(header) + "\n"

	visibleRows := d.getVisibleRows()

	// Calculate scroll offset to keep selected row visible
	scrollOffset := 0
	if d.selectedRow >= visibleRows {
		scrollOffset = d.selectedRow - visibleRows + 1
	}

	endRow := scrollOffset + visibleRows
	if endRow > len(d.properties) {
		endRow = len(d.properties)
	}

	for i := scrollOffset; i < endRow; i++ {
		p := d.properties[i]
		price := "—"
		if p.LatestPrice > 0 {
			price = fmt.Sprintf("$%d", p.LatestPrice/1000) + "K"
		}
		synced := styles.StatusPending.Render("○")
		if p.Synced {
			synced = styles.StatusSuccess.Render("✓")
		}

		row := fmt.Sprintf("%-35s %-12s %10s %4d %4d %7s %-8s %5d %6s",
			truncate(p.NormalizedAddress, 35),
			truncate(p.City, 12),
			price,
			p.Beds,
			p.Baths,
			formatSqft(p.Sqft),
			truncate(p.PropertyType, 8),
			p.TimesListed,
			synced,
		)

		if i == d.selectedRow {
			rows += styles.TableSelected.Render(row) + "\n"
		} else {
			rows += row + "\n"
		}
	}

	// Show scroll indicator
	if len(d.properties) > visibleRows {
		rows += styles.Muted.Render(fmt.Sprintf("  [%d-%d of %d]", scrollOffset+1, endRow, len(d.properties)))
	}

	return rows
}

func (d Data) renderBottomPanel() string {
	priceHistory := d.renderPriceHistory()
	listingDetails := d.renderListingDetails()

	historyBox := styles.CardBorder.Width(d.width/2 - 2).Render(
		styles.Title.Render("Price History") + "\n" + priceHistory,
	)
	detailsBox := styles.SiteCardBorder.Width(d.width/2 - 2).Render(
		styles.Title.Render("Listing Details") + "\n" + listingDetails,
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, historyBox, detailsBox)
}

func (d Data) renderPriceHistory() string {
	if len(d.snapshots) == 0 {
		return styles.Muted.Render("Select a property")
	}

	header := fmt.Sprintf("%-12s %-10s %12s", "Date", "Site", "Price")
	rows := styles.TableHeader.Render(header) + "\n"

	maxRows := 8
	if len(d.snapshots) < maxRows {
		maxRows = len(d.snapshots)
	}

	var prevPrice int
	for i := 0; i < maxRows; i++ {
		s := d.snapshots[i]
		date := s.ScrapedAt.Format("2006-01-02")
		price := fmt.Sprintf("$%d", s.Price/1000) + "K"

		priceStyle := lipgloss.NewStyle()
		if i > 0 && prevPrice > 0 && s.Price != prevPrice {
			if s.Price > prevPrice {
				priceStyle = styles.StatusError
			} else {
				priceStyle = styles.StatusSuccess
			}
		}
		prevPrice = s.Price

		row := fmt.Sprintf("%-12s %-10s %12s",
			date,
			truncate(s.SiteID, 10),
			priceStyle.Render(price),
		)
		rows += row + "\n"
	}
	return rows
}

func (d Data) renderListingDetails() string {
	if len(d.snapshots) == 0 {
		return styles.Muted.Render("Select a property")
	}

	s := d.snapshots[0]
	lines := []string{
		fmt.Sprintf("MLS#: %s", s.ListingID),
		"",
	}

	if s.Description != "" {
		desc := s.Description
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		wrapped := wrapText(desc, d.width/2-6)
		lines = append(lines, wrapped...)
		lines = append(lines, "")
	}

	if s.Realtor.AgentName != "" {
		lines = append(lines, styles.StatLabel.Render("Agent: ")+s.Realtor.AgentName)
	}
	if s.Realtor.AgentPhone != "" {
		lines = append(lines, styles.StatLabel.Render("Phone: ")+s.Realtor.AgentPhone)
	}
	if s.Realtor.CompanyName != "" {
		lines = append(lines, styles.StatLabel.Render("Company: ")+s.Realtor.CompanyName)
	}

	lines = append(lines, "", styles.Muted.Render(truncate(s.URL, d.width/2-6)))

	return strings.Join(lines, "\n")
}

func formatSqft(sqft int) string {
	if sqft == 0 {
		return "—"
	}
	if sqft >= 1000 {
		return fmt.Sprintf("%d,%03d", sqft/1000, sqft%1000)
	}
	return fmt.Sprintf("%d", sqft)
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		width = 40
	}
	var lines []string
	words := strings.Fields(text)
	var line string
	for _, word := range words {
		if len(line)+len(word)+1 > width {
			lines = append(lines, line)
			line = word
		} else {
			if line != "" {
				line += " "
			}
			line += word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}
