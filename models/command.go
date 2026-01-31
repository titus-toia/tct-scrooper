package models

import (
	"encoding/json"
	"time"
)

type CommandType string

const (
	CmdScrapeNow  CommandType = "scrape_now"
	CmdScrapeSite CommandType = "scrape_site"
	CmdPause      CommandType = "pause"
	CmdResume     CommandType = "resume"
)

type Command struct {
	ID          int64           `json:"id" db:"id"`
	Command     CommandType     `json:"command" db:"command"`
	Params      json.RawMessage `json:"params" db:"params"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	ProcessedAt *time.Time      `json:"processed_at" db:"processed_at"`
}

type CommandParams struct {
	Site   string `json:"site,omitempty"`
	Region string `json:"region,omitempty"`
}
