"""Data viewer for properties and snapshots."""
from textual.app import ComposeResult
from textual.containers import Container, Vertical, Horizontal
from textual.widgets import Static, DataTable, Switch, Label
from textual.message import Message

from ..clients.db_client import DatabaseClient


class DataView(Container):
	"""Properties and snapshots data viewer."""

	def __init__(self, db: DatabaseClient):
		super().__init__()
		self.db = db
		self.unsynced_only = False
		self.selected_property_id = None
		self.snapshots = []

	def compose(self) -> ComposeResult:
		with Vertical(id="data-main"):
			yield Static("[bold]Properties[/bold]", classes="section-title")

			with Horizontal(id="data-filters"):
				yield Label("Unsynced only: ")
				yield Switch(id="unsynced-toggle", value=False)

			yield DataTable(id="properties-table", cursor_type="row")

			yield Static("[bold]Price History[/bold]", classes="section-title")
			yield DataTable(id="snapshots-table")

	def on_mount(self) -> None:
		self.setup_tables()
		self.refresh_properties()

	def setup_tables(self) -> None:
		props_table = self.query_one("#properties-table", DataTable)
		props_table.add_columns("Address", "City", "Price", "Beds", "Baths", "SqFt", "Type", "Listed", "Synced")

		snaps_table = self.query_one("#snapshots-table", DataTable)
		snaps_table.add_columns("Date", "Site", "Price", "URL")

	def on_switch_changed(self, event: Switch.Changed) -> None:
		if event.switch.id == "unsynced-toggle":
			self.unsynced_only = event.value
			self.refresh_properties()

	def on_data_table_row_highlighted(self, event: DataTable.RowHighlighted) -> None:
		if event.data_table.id == "properties-table":
			props = self.db.get_properties(limit=100, unsynced_only=self.unsynced_only)
			if event.cursor_row is not None and event.cursor_row < len(props):
				self.selected_property_id = props[event.cursor_row].id
				self.refresh_snapshots()

	def refresh_properties(self) -> None:
		props = self.db.get_properties(limit=100, unsynced_only=self.unsynced_only)
		table = self.query_one("#properties-table", DataTable)
		table.clear()

		for prop in props:
			synced_icon = "[green]✓[/green]" if prop.synced else "[yellow]○[/yellow]"
			price = f"${prop.latest_price:,}" if prop.latest_price else "-"
			table.add_row(
				prop.normalized_address[:40] if prop.normalized_address else "-",
				prop.city or "-",
				price,
				str(prop.beds),
				str(prop.baths),
				f"{prop.sqft:,}" if prop.sqft else "-",
				prop.property_type or "-",
				str(prop.times_listed),
				synced_icon,
			)

	def refresh_snapshots(self) -> None:
		if not self.selected_property_id:
			return

		self.snapshots = self.db.get_snapshots_for_property(self.selected_property_id)
		table = self.query_one("#snapshots-table", DataTable)
		table.clear()

		for snap in self.snapshots:
			date = snap.scraped_at.strftime("%Y-%m-%d %H:%M") if snap.scraped_at else "-"
			price = f"${snap.price:,}" if snap.price else "-"
			url = snap.url[:50] + "..." if snap.url and len(snap.url) > 50 else snap.url or "-"
			table.add_row(date, snap.site_id, price, url)

	def get_selected_snapshot_url(self) -> str | None:
		table = self.query_one("#snapshots-table", DataTable)
		if table.cursor_row is not None and table.cursor_row < len(self.snapshots):
			return self.snapshots[table.cursor_row].url
		return None
