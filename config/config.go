package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Proxy     ProxyConfig
	Supabase  SupabaseConfig
	Scheduler SchedulerConfig
	Scraper   ScraperConfig
	MediaS3   MediaS3Config
	DBPath    string
	LogLevel  string
	Sites     map[string]*SiteConfig
}

type MediaS3Config struct {
	Bucket          string
	Region          string
	Endpoint        string // Optional: for DO Spaces, R2, etc.
	AccessKeyID     string
	SecretAccessKey string
}

type ProxyConfig struct {
	URL string
}

type SupabaseConfig struct {
	DBURL string // Direct Postgres connection string
}

type SchedulerConfig struct {
	Interval time.Duration
	Cron     string
}

type ScraperConfig struct {
	DelayMS int
}

type SiteConfig struct {
	ID               string            `yaml:"id"`
	Name             string            `yaml:"name"`
	Handler          string            `yaml:"handler"`
	RateLimitMS      int               `yaml:"rate_limit_ms"`
	Endpoints        map[string]string `yaml:"endpoints"`
	Regions          map[string]Region `yaml:"regions"`
	ApifyActor       string            `yaml:"apify_actor"`
	ApifyMaxListings int               `yaml:"apify_max_listings"`
}

type Region struct {
	Slug    string  `yaml:"slug"`
	GeoID   string  `yaml:"geo_id"`
	GeoName string  `yaml:"geo_name"`
	LatMin  float64 `yaml:"lat_min"`
	LatMax  float64 `yaml:"lat_max"`
	LngMin  float64 `yaml:"lng_min"`
	LngMax  float64 `yaml:"lng_max"`
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Proxy: ProxyConfig{
			URL: os.Getenv("PROXY_URL"),
		},
		Supabase: SupabaseConfig{
			DBURL: os.Getenv("SUPABASE_DB_URL"),
		},
		Scheduler: SchedulerConfig{
			Cron: os.Getenv("SCRAPE_CRON"),
		},
		Scraper: ScraperConfig{
			DelayMS: getEnvInt("SCRAPE_DELAY_MS", 500),
		},
		MediaS3: MediaS3Config{
			Bucket:          os.Getenv("MEDIA_S3_BUCKET"),
			Region:          os.Getenv("MEDIA_S3_REGION"),
			Endpoint:        os.Getenv("MEDIA_S3_ENDPOINT"),
			AccessKeyID:     os.Getenv("MEDIA_S3_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("MEDIA_S3_SECRET_ACCESS_KEY"),
		},
		DBPath: getEnv("DB_PATH", "scraper.db"),
		LogLevel: getEnv("LOG_LEVEL", "info"),
		Sites:    make(map[string]*SiteConfig),
	}

	if interval := os.Getenv("SCRAPE_INTERVAL"); interval != "" {
		d, err := time.ParseDuration(interval)
		if err == nil {
			cfg.Scheduler.Interval = d
		}
	}

	if err := cfg.loadSiteConfigs(); err != nil {
		return nil, err
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	var missing []string

	if c.Proxy.URL == "" {
		missing = append(missing, "PROXY_URL")
	}

	// Supabase DB URL is required for domain data (Postgres)
	if c.Supabase.DBURL == "" {
		missing = append(missing, "SUPABASE_DB_URL (required for Postgres)")
	}

	// Check for Apify key if any site uses apify handler
	apifyKey := os.Getenv("APIFY_API_KEY")
	for _, site := range c.Sites {
		if site.Handler == "apify" && apifyKey == "" {
			missing = append(missing, "APIFY_API_KEY (required for apify handler)")
			break
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required config:\n  - %s", joinStrings(missing, "\n  - "))
	}

	return nil
}

// HasPostgres returns true if Postgres connection is configured
func (c *Config) HasPostgres() bool {
	return c.Supabase.DBURL != ""
}

// HasMediaS3 returns true if S3 media storage is configured
func (c *Config) HasMediaS3() bool {
	return c.MediaS3.Bucket != "" && c.MediaS3.AccessKeyID != "" && c.MediaS3.SecretAccessKey != ""
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += sep + s
	}
	return result
}

func (c *Config) loadSiteConfigs() error {
	configDir := "config/sites"
	entries, err := os.ReadDir(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		path := filepath.Join(configDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var site SiteConfig
		if err := yaml.Unmarshal(data, &site); err != nil {
			return err
		}

		c.Sites[site.ID] = &site
	}

	return nil
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
