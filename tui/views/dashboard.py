"""Dashboard view showing overall stats."""
from textual.app import ComposeResult
from textual.containers import Container, Horizontal, Vertical
from textual.widgets import Static, DataTable, Label
from textual.reactive import reactive

from ..clients.db_client import DatabaseClient


class StatCard(Static):
	"""A single stat card with label and value."""

	def __init__(self, label: str, value: str = "0", variant: str = "default"):
		super().__init__()
		self.stat_label = label
		self.stat_value = value
		self.variant = variant

	def compose(self) -> ComposeResult:
		yield Static(self.stat_value, classes="stat-value")
		yield Static(self.stat_label, classes="stat-label")

	def update_value(self, value: str) -> None:
		self.stat_value = value
		self.query_one(".stat-value", Static).update(value)


class SiteCard(Static):
	"""Card showing site status."""

	def __init__(self, site_id: str, status: str, properties: int, success_rate: float):
		super().__init__()
		self.site_id = site_id
		self.status = status
		self.properties = properties
		self.success_rate = success_rate

	def compose(self) -> ComposeResult:
		status_class = "success" if self.status == "completed" else "error" if self.status == "failed" else "pending"
		yield Static(f"[bold]{self.site_id}[/bold]", classes="site-name")
		yield Static(f"{self.status or 'never run'}", classes=f"site-status {status_class}")
		yield Static(f"{self.properties:,} properties", classes="site-props")
		yield Static(f"{self.success_rate:.0%} success", classes="site-rate")


class Dashboard(Container):
	"""Main dashboard view."""

	def __init__(self, db: DatabaseClient):
		super().__init__()
		self.db = db

	def compose(self) -> ComposeResult:
		with Vertical(id="dashboard-main"):
			yield Static("[bold]Dashboard[/bold]", classes="section-title")

			with Horizontal(id="stats-row"):
				yield StatCard("Total Properties", "0", "primary")
				yield StatCard("Total Snapshots", "0", "info")
				yield StatCard("Unsynced", "0", "warning")
				yield StatCard("Sites", "0", "default")

			yield Static("[bold]Sites[/bold]", classes="section-title")
			with Horizontal(id="sites-row"):
				pass  # Sites will be added on refresh

			yield Static("[bold]Recent Runs[/bold]", classes="section-title")
			yield DataTable(id="runs-table")

	def on_mount(self) -> None:
		self.refresh_data()

	def refresh_data(self) -> None:
		stats = self.db.get_site_stats()
		runs = self.db.get_recent_runs(10)

		total_props = self.db.get_property_count()
		total_snaps = self.db.get_snapshot_count()
		unsynced = len(self.db.get_properties(limit=1000, unsynced_only=True))

		stat_cards = self.query("#stats-row StatCard").results()
		stat_cards = list(stat_cards)
		if len(stat_cards) >= 4:
			stat_cards[0].update_value(f"{total_props:,}")
			stat_cards[1].update_value(f"{total_snaps:,}")
			stat_cards[2].update_value(f"{unsynced:,}")
			stat_cards[3].update_value(str(len(stats)))

		sites_row = self.query_one("#sites-row")
		sites_row.remove_children()
		for stat in stats:
			sites_row.mount(SiteCard(
				site_id=stat.site_id,
				status=stat.last_run_status,
				properties=stat.total_properties,
				success_rate=stat.success_rate,
			))

		table = self.query_one("#runs-table", DataTable)
		table.clear(columns=True)
		table.add_columns("Site", "Status", "Started", "Found", "New", "Errors")
		for run in runs:
			started = run.started_at.strftime("%H:%M:%S") if run.started_at else "-"
			table.add_row(
				run.site_id,
				run.status,
				started,
				str(run.listings_found),
				str(run.properties_new),
				str(run.errors_count),
			)
