package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ExpressVPN   ExpressVPNConfig
	Supabase     SupabaseConfig
	Scheduler    SchedulerConfig
	Scraper      ScraperConfig
	DBPath       string
	LogLevel     string
	Sites        map[string]*SiteConfig
}

type ExpressVPNConfig struct {
	ActivationCode string
	AutoConnect    bool
	Region         string
}

type SupabaseConfig struct {
	URL        string
	AnonKey    string
	ServiceKey string
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
		ExpressVPN: ExpressVPNConfig{
			ActivationCode: os.Getenv("EXPRESSVPN_ACTIVATION_CODE"),
			AutoConnect:    os.Getenv("EXPRESSVPN_AUTOCONNECT") == "true",
			Region:         getEnv("EXPRESSVPN_REGION", "smart"),
		},
		Supabase: SupabaseConfig{
			URL:        os.Getenv("SUPABASE_URL"),
			AnonKey:    os.Getenv("SUPABASE_ANON_KEY"),
			ServiceKey: os.Getenv("SUPABASE_SERVICE_KEY"),
		},
		Scheduler: SchedulerConfig{
			Cron: os.Getenv("SCRAPE_CRON"),
		},
		Scraper: ScraperConfig{
			DelayMS: getEnvInt("SCRAPE_DELAY_MS", 500),
		},
		DBPath:   getEnv("DB_PATH", "scraper.db"),
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

	return cfg, nil
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
