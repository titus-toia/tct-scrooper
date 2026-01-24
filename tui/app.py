"""
TCT Scrooper TUI - Terminal Admin Interface
"""
import os
from textual.app import App, ComposeResult
from textual.binding import Binding
from textual.widgets import Header, Footer, TabbedContent, TabPane, Static
from textual.containers import Container

from .clients.db_client import DatabaseClient
from .views.dashboard import Dashboard
from .views.logs import LogsView
from .views.data import DataView


class ScrooperTUI(App):
	"""Main TUI application for TCT Scrooper."""

	CSS = """
	Screen {
		background: $surface;
	}

	#main-content {
		height: 100%;
	}

	.section-title {
		padding: 1 2;
		text-style: bold;
		color: $primary;
	}

	#stats-row {
		height: auto;
		padding: 1;
	}

	StatCard {
		width: 1fr;
		height: 5;
		border: solid $primary;
		padding: 0 2;
		margin: 0 1;
	}

	StatCard .stat-value {
		text-align: center;
		text-style: bold;
		color: $accent;
	}

	StatCard .stat-label {
		text-align: center;
		color: $text-muted;
	}

	#sites-row {
		height: auto;
		padding: 1;
	}

	SiteCard {
		width: 1fr;
		height: 6;
		border: solid $secondary;
		padding: 0 2;
		margin: 0 1;
	}

	SiteCard .site-name {
		text-align: center;
	}

	SiteCard .site-status {
		text-align: center;
	}

	SiteCard .site-status.success {
		color: $success;
	}

	SiteCard .site-status.error {
		color: $error;
	}

	SiteCard .site-status.pending {
		color: $warning;
	}

	SiteCard .site-props, SiteCard .site-rate {
		text-align: center;
		color: $text-muted;
	}

	#runs-table {
		height: 12;
		margin: 1;
	}

	#logs-main {
		height: 100%;
	}

	#level-filter {
		width: 30;
		margin: 0 2 1 2;
	}

	#log-output {
		height: 1fr;
		margin: 0 2;
		border: solid $primary;
		scrollbar-gutter: stable;
	}

	#data-main {
		height: 100%;
	}

	#data-filters {
		height: auto;
		padding: 1 2;
	}

	#properties-table {
		height: 50%;
		margin: 1;
	}

	#snapshots-table {
		height: 30%;
		margin: 1;
	}

	#command-bar {
		dock: bottom;
		height: 3;
		padding: 0 2;
		background: $surface-darken-1;
	}

	.command-hint {
		color: $text-muted;
	}
	"""

	BINDINGS = [
		Binding("q", "quit", "Quit"),
		Binding("r", "refresh", "Refresh"),
		Binding("d", "show_tab('dashboard')", "Dashboard", show=False),
		Binding("l", "show_tab('logs')", "Logs", show=False),
		Binding("p", "show_tab('data')", "Properties", show=False),
		Binding("s", "scrape_now", "Scrape Now"),
		Binding("y", "sync_now", "Sync"),
		Binding("c", "copy_url", "Copy URL"),
	]

	TITLE = "TCT Scrooper"
	SUB_TITLE = "Property Scraper Admin"

	def __init__(self):
		super().__init__()
		db_path = os.environ.get("DB_PATH", "scraper.db")
		self.db = DatabaseClient(db_path)

	def compose(self) -> ComposeResult:
		yield Header()
		with TabbedContent(id="main-content"):
			with TabPane("Dashboard", id="dashboard"):
				yield Dashboard(self.db)
			with TabPane("Logs", id="logs"):
				yield LogsView(self.db)
			with TabPane("Data", id="data"):
				yield DataView(self.db)
		yield Static(
			"[dim]r[/dim] Refresh  [dim]s[/dim] Scrape Now  [dim]y[/dim] Sync  [dim]c[/dim] Copy URL  [dim]q[/dim] Quit",
			id="command-bar"
		)
		yield Footer()

	def on_mount(self) -> None:
		self.db.connect()
		self.set_interval(30, self.action_refresh)

	def on_unmount(self) -> None:
		self.db.close()

	def action_refresh(self) -> None:
		active_tab = self.query_one(TabbedContent).active
		if active_tab == "dashboard":
			self.query_one(Dashboard).refresh_data()
		elif active_tab == "logs":
			self.query_one(LogsView).refresh_logs()
		elif active_tab == "data":
			self.query_one(DataView).refresh_properties()
		self.notify("Refreshed", timeout=1)

	def action_show_tab(self, tab_id: str) -> None:
		self.query_one(TabbedContent).active = tab_id

	def action_scrape_now(self) -> None:
		self.db.scrape_now()
		self.notify("Scrape command sent!", severity="information")

	def action_sync_now(self) -> None:
		self.db.sync_now()
		self.notify("Sync command sent!", severity="information")

	def action_copy_url(self) -> None:
		data_view = self.query_one(DataView)
		url = data_view.get_selected_snapshot_url()
		if url:
			self.copy_to_clipboard(url)
			self.notify("URL copied!", timeout=2)
		else:
			self.notify("No URL selected", severity="warning", timeout=2)


def main():
	app = ScrooperTUI()
	app.run()


if __name__ == "__main__":
	main()
