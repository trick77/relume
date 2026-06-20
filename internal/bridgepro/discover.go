package bridgepro

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// ErrCloudRateLimited means discovery.meethue.com answered HTTP 429 (Too Many
// Requests): the Philips cloud is throttling our polling, not a real outage and not a
// statement about whether a bridge exists (when not throttled it returns an empty list
// if no bridge is present). Callers should back off — honoring RateLimitedError.RetryAfter
// — and keep waiting rather than reporting "cloud unavailable".
var ErrCloudRateLimited = errors.New("cloud discovery rate-limited")

// RateLimitedError carries the server's Retry-After hint so callers can back off for at
// least that long (the Philips cloud's retry-after can be many minutes; polling sooner
// just extends the throttle). Unwraps to ErrCloudRateLimited for errors.Is checks.
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("cloud discovery rate-limited (retry after %s)", e.RetryAfter)
}

func (e *RateLimitedError) Unwrap() error { return ErrCloudRateLimited }

// ModelHueBridgePro is the modelid a real Hue Bridge Pro reports in its
// /api/0/config. relumeTV only drives a Pro, so a discovered bridge whose modelid
// differs is rejected as "not a Pro". NOTE: this string is matched against the real
// hardware during rollout — if it is wrong, EVERY Pro is rejected, so any mismatch
// must surface the actual observed modelid loudly (see FetchModelID callers).
const ModelHueBridgePro = "BSB003"

// FetchModelID reads the unauthenticated short config (GET https://<host>/api/0/config)
// and returns the bridge's modelid. No app key is needed (this endpoint is open) and
// no certificate is pinned yet at discovery time, so TLS verification is skipped — the
// same posture as FetchLeafFingerprint, which runs at the same pre-pairing stage.
func FetchModelID(host string) (string, error) {
	client := &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // pre-pairing discovery, no cert pinned yet
		},
	}
	resp, err := client.Get("https://" + host + "/api/0/config")
	if err != nil {
		return "", fmt.Errorf("fetch modelid from %s: %w", host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch modelid from %s: status %d", host, resp.StatusCode)
	}
	var cfg struct {
		ModelID string `json:"modelid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", fmt.Errorf("parse modelid from %s: %w", host, err)
	}
	return cfg.ModelID, nil
}

// DiscoveredBridge is an entry from the Philips cloud discovery.
type DiscoveredBridge struct {
	ID                string `json:"id"`
	InternalIPAddress string `json:"internalipaddress"`
	Port              int    `json:"port"`
}

// Discover queries the Philips cloud discovery (discovery.meethue.com) for bridges
// on the same public network. Requires internet access; returns the local IPs.
func Discover() ([]DiscoveredBridge, error) {
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get("https://discovery.meethue.com/")
	if err != nil {
		return nil, fmt.Errorf("cloud discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		// Philips throttles discovery.meethue.com aggressively (Retry-After can be many
		// minutes). Surface it as a distinct, non-alarming error carrying the hint.
		return nil, &RateLimitedError{RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloud discovery: status %d", resp.StatusCode)
	}
	var bridges []DiscoveredBridge
	if err := json.NewDecoder(resp.Body).Decode(&bridges); err != nil {
		return nil, fmt.Errorf("parse cloud discovery: %w", err)
	}
	return bridges, nil
}

// parseRetryAfter reads a Retry-After header (delta-seconds form, which the Philips
// cloud uses, e.g. "54"). Falls back to a conservative default when absent or
// unparseable, and clamps to a sane ceiling so a pathological value can't stall setup.
func parseRetryAfter(h string) time.Duration {
	const fallback = 60 * time.Second
	const ceiling = 5 * time.Minute
	if h == "" {
		return fallback
	}
	secs, err := strconv.Atoi(h)
	if err != nil || secs <= 0 {
		return fallback
	}
	d := time.Duration(secs) * time.Second
	if d > ceiling {
		return ceiling
	}
	return d
}
