package workers

import "tct_scrooper/models"

// LogFunc is a function that logs to the scrape_logs table
type LogFunc func(level models.LogLevel, source, message string)

// NoOpLogger does nothing (default)
var NoOpLogger LogFunc = func(level models.LogLevel, source, message string) {}
