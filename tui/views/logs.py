"""Logs viewer with live updates."""
from textual.app import ComposeResult
from textual.containers import Container, Vertical, ScrollableContainer
from textual.widgets import Static, RichLog, Select
from textual.message import Message

from ..clients.db_client import DatabaseClient, ScrapeLog


class LogLine(Static):
	"""Single log line with level-based styling."""

	def __init__(self, log: ScrapeLog):
		self.log = log
		super().__init__(self._format())

	def _format(self) -> str:
		level_colors = {
			"DEBUG": "dim",
			"INFO": "green",
			"WARN": "yellow",
			"ERROR": "red bold",
			"FATAL": "red bold reverse",
		}
		color = level_colors.get(self.log.level, "white")
		ts = self.log.timestamp.strftime("%H:%M:%S") if self.log.timestamp else "--:--:--"
		site = f"[dim]{self.log.site_id}[/dim] " if self.log.site_id else ""
		return f"[dim]{ts}[/dim] [{color}]{self.log.level:5}[/{color}] {site}{self.log.message}"


class LogsView(Container):
	"""Logs viewer with filtering."""

	def __init__(self, db: DatabaseClient):
		super().__init__()
		self.db = db
		self.level_filter = None

	def compose(self) -> ComposeResult:
		with Vertical(id="logs-main"):
			yield Static("[bold]Logs[/bold]", classes="section-title")
			yield Select(
				[("All Levels", "ALL"), ("Debug", "DEBUG"), ("Info", "INFO"), ("Warning", "WARN"), ("Error", "ERROR")],
				prompt="Filter by level",
				id="level-filter",
				value="ALL",
			)
			yield RichLog(id="log-output", highlight=True, markup=True)

	def on_mount(self) -> None:
		self.refresh_logs()

	def on_select_changed(self, event: Select.Changed) -> None:
		if event.select.id == "level-filter":
			self.level_filter = event.value if event.value != "ALL" else None
			self.refresh_logs()

	def refresh_logs(self) -> None:
		logs = self.db.get_recent_logs(limit=200, level=self.level_filter)
		log_widget = self.query_one("#log-output", RichLog)
		log_widget.clear()

		level_colors = {
			"DEBUG": "dim white",
			"INFO": "green",
			"WARN": "yellow",
			"ERROR": "red",
			"FATAL": "red bold",
		}

		for log in reversed(logs):
			color = level_colors.get(log.level, "white")
			ts = log.timestamp.strftime("%H:%M:%S") if log.timestamp else "--:--:--"
			site = f"[dim]{log.site_id}[/dim] " if log.site_id else ""
			log_widget.write(f"[dim]{ts}[/dim] [{color}]{log.level:5}[/{color}] {site}{log.message}")
