package httputil

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"time"

	"tct_scrooper/config"
)

type Clients struct {
	Scraping *http.Client // proxied, for target sites
	API      *http.Client // direct, for Apify/Supabase
}

func NewClients(proxyCfg *config.ProxyConfig) *Clients {
	proxyURL, _ := url.Parse(proxyCfg.URL)

	transport := &http.Transport{
		Proxy:             http.ProxyURL(proxyURL),
		ForceAttemptHTTP2: false,
		TLSNextProto:      make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}

	scraping := &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return &Clients{
		Scraping: scraping,
		API:      &http.Client{Timeout: 30 * time.Second},
	}
}
