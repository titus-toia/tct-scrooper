package models

import "time"

type RunStatus string

const (
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
)

type ScrapeRun struct {
	ID                 int64      `json:"id" db:"id"`
	SiteID             string     `json:"site_id" db:"site_id"`
	StartedAt          time.Time  `json:"started_at" db:"started_at"`
	FinishedAt         *time.Time `json:"finished_at" db:"finished_at"`
	Status             RunStatus  `json:"status" db:"status"`
	ListingsFound      int        `json:"listings_found" db:"listings_found"`
	ListingsNew        int        `json:"listings_new" db:"listings_new"`
	PropertiesNew      int        `json:"properties_new" db:"properties_new"`
	PropertiesRelisted int        `json:"properties_relisted" db:"properties_relisted"`
	ErrorsCount        int        `json:"errors_count" db:"errors_count"`
}

type SiteStats struct {
	SiteID            string     `json:"site_id" db:"site_id"`
	LastRunAt         *time.Time `json:"last_run_at" db:"last_run_at"`
	LastRunStatus     string     `json:"last_run_status" db:"last_run_status"`
	TotalProperties   int        `json:"total_properties" db:"total_properties"`
	TotalSnapshots    int        `json:"total_snapshots" db:"total_snapshots"`
	PropertiesSynced  int        `json:"properties_synced" db:"properties_synced"`
	SuccessRate       float64    `json:"success_rate" db:"success_rate"`
	AvgRunDurationSec int        `json:"avg_run_duration_sec" db:"avg_run_duration_sec"`
}
