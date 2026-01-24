package models

import "time"

type LogLevel string

const (
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

type ScrapeLog struct {
	ID        int64     `json:"id" db:"id"`
	RunID     *int64    `json:"run_id" db:"run_id"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	Level     LogLevel  `json:"level" db:"level"`
	Message   string    `json:"message" db:"message"`
	SiteID    string    `json:"site_id" db:"site_id"`
}
