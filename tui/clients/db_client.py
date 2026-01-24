"""
SQLite client for TUI - reads scraper data and writes commands.
"""
import sqlite3
import json
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Optional


@dataclass
class SiteStats:
	site_id: str
	last_run_at: Optional[datetime]
	last_run_status: Optional[str]
	total_properties: int
	total_snapshots: int
	properties_synced: int
	success_rate: float
	avg_run_duration_sec: int


@dataclass
class ScrapeRun:
	id: int
	site_id: str
	started_at: datetime
	finished_at: Optional[datetime]
	status: str
	listings_found: int
	listings_new: int
	properties_new: int
	properties_relisted: int
	errors_count: int


@dataclass
class ScrapeLog:
	id: int
	run_id: Optional[int]
	timestamp: datetime
	level: str
	message: str
	site_id: Optional[str]


@dataclass
class Property:
	id: str
	normalized_address: str
	city: str
	beds: int
	baths: int
	sqft: int
	property_type: str
	first_seen_at: datetime
	last_seen_at: datetime
	times_listed: int
	synced: bool
	latest_price: int


@dataclass
class Snapshot:
	id: int
	property_id: str
	listing_id: str
	site_id: str
	url: str
	price: int
	data: dict
	scraped_at: datetime
	run_id: int


class DatabaseClient:
	def __init__(self, db_path: str = "scraper.db"):
		self.db_path = Path(db_path)
		self._conn: Optional[sqlite3.Connection] = None

	def connect(self) -> None:
		self._conn = sqlite3.connect(
			str(self.db_path),
			check_same_thread=False,
			timeout=5.0
		)
		self._conn.row_factory = sqlite3.Row

	def close(self) -> None:
		if self._conn:
			self._conn.close()
			self._conn = None

	@property
	def conn(self) -> sqlite3.Connection:
		if not self._conn:
			self.connect()
		return self._conn

	def get_site_stats(self) -> list[SiteStats]:
		cursor = self.conn.execute("""
			SELECT site_id, last_run_at, last_run_status, total_properties,
				total_snapshots, properties_synced, success_rate, avg_run_duration_sec
			FROM site_stats ORDER BY site_id
		""")
		return [
			SiteStats(
				site_id=row["site_id"],
				last_run_at=self._parse_datetime(row["last_run_at"]),
				last_run_status=row["last_run_status"],
				total_properties=row["total_properties"] or 0,
				total_snapshots=row["total_snapshots"] or 0,
				properties_synced=row["properties_synced"] or 0,
				success_rate=row["success_rate"] or 0.0,
				avg_run_duration_sec=row["avg_run_duration_sec"] or 0,
			)
			for row in cursor.fetchall()
		]

	def get_recent_runs(self, limit: int = 20) -> list[ScrapeRun]:
		cursor = self.conn.execute("""
			SELECT id, site_id, started_at, finished_at, status, listings_found,
				listings_new, properties_new, properties_relisted, errors_count
			FROM scrape_runs ORDER BY started_at DESC LIMIT ?
		""", (limit,))
		return [
			ScrapeRun(
				id=row["id"],
				site_id=row["site_id"],
				started_at=self._parse_datetime(row["started_at"]),
				finished_at=self._parse_datetime(row["finished_at"]),
				status=row["status"],
				listings_found=row["listings_found"] or 0,
				listings_new=row["listings_new"] or 0,
				properties_new=row["properties_new"] or 0,
				properties_relisted=row["properties_relisted"] or 0,
				errors_count=row["errors_count"] or 0,
			)
			for row in cursor.fetchall()
		]

	def get_recent_logs(self, limit: int = 100, level: Optional[str] = None) -> list[ScrapeLog]:
		if level:
			cursor = self.conn.execute("""
				SELECT id, run_id, timestamp, level, message, site_id
				FROM scrape_logs WHERE level = ? ORDER BY timestamp DESC LIMIT ?
			""", (level, limit))
		else:
			cursor = self.conn.execute("""
				SELECT id, run_id, timestamp, level, message, site_id
				FROM scrape_logs ORDER BY timestamp DESC LIMIT ?
			""", (limit,))
		return [
			ScrapeLog(
				id=row["id"],
				run_id=row["run_id"],
				timestamp=self._parse_datetime(row["timestamp"]),
				level=row["level"],
				message=row["message"],
				site_id=row["site_id"],
			)
			for row in cursor.fetchall()
		]

	def get_properties(self, limit: int = 100, unsynced_only: bool = False) -> list[Property]:
		where_clause = "WHERE p.synced = FALSE" if unsynced_only else ""
		cursor = self.conn.execute(f"""
			SELECT p.id, p.normalized_address, p.city, p.beds, p.baths, p.sqft, p.property_type,
				p.first_seen_at, p.last_seen_at, p.times_listed, p.synced,
				(SELECT ls.price FROM listing_snapshots ls
				 WHERE ls.property_id = p.id ORDER BY ls.scraped_at DESC LIMIT 1) as latest_price
			FROM properties p {where_clause} ORDER BY p.last_seen_at DESC LIMIT ?
		""", (limit,))
		return [
			Property(
				id=row["id"],
				normalized_address=row["normalized_address"],
				city=row["city"],
				beds=row["beds"] or 0,
				baths=row["baths"] or 0,
				sqft=row["sqft"] or 0,
				property_type=row["property_type"],
				first_seen_at=self._parse_datetime(row["first_seen_at"]),
				last_seen_at=self._parse_datetime(row["last_seen_at"]),
				times_listed=row["times_listed"] or 1,
				synced=bool(row["synced"]),
				latest_price=row["latest_price"] or 0,
			)
			for row in cursor.fetchall()
		]

	def get_property_count(self) -> int:
		cursor = self.conn.execute("SELECT COUNT(*) FROM properties")
		return cursor.fetchone()[0]

	def get_snapshot_count(self) -> int:
		cursor = self.conn.execute("SELECT COUNT(*) FROM listing_snapshots")
		return cursor.fetchone()[0]

	def get_snapshots_for_property(self, property_id: str) -> list[Snapshot]:
		cursor = self.conn.execute("""
			SELECT id, property_id, listing_id, site_id, url, price, data, scraped_at, run_id
			FROM listing_snapshots WHERE property_id = ? ORDER BY scraped_at DESC
		""", (property_id,))
		return [
			Snapshot(
				id=row["id"],
				property_id=row["property_id"],
				listing_id=row["listing_id"],
				site_id=row["site_id"],
				url=row["url"],
				price=row["price"] or 0,
				data=json.loads(row["data"]) if row["data"] else {},
				scraped_at=self._parse_datetime(row["scraped_at"]),
				run_id=row["run_id"],
			)
			for row in cursor.fetchall()
		]

	def send_command(self, command: str, params: Optional[dict] = None) -> int:
		cursor = self.conn.execute("""
			INSERT INTO commands (command, params, created_at)
			VALUES (?, ?, ?)
		""", (command, json.dumps(params) if params else None, datetime.now()))
		self.conn.commit()
		return cursor.lastrowid

	def scrape_now(self) -> int:
		return self.send_command("scrape_now")

	def scrape_site(self, site_id: str) -> int:
		return self.send_command("scrape_site", {"site": site_id})

	def pause(self) -> int:
		return self.send_command("pause")

	def resume(self) -> int:
		return self.send_command("resume")

	def sync_now(self) -> int:
		return self.send_command("sync_now")

	def _parse_datetime(self, value) -> Optional[datetime]:
		if not value:
			return None
		if isinstance(value, datetime):
			return value
		try:
			return datetime.fromisoformat(value.replace("Z", "+00:00"))
		except (ValueError, AttributeError):
			return None
