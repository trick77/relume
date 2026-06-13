package bridgepro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cloud discovery: status %d", resp.StatusCode)
	}
	var bridges []DiscoveredBridge
	if err := json.NewDecoder(resp.Body).Decode(&bridges); err != nil {
		return nil, fmt.Errorf("parse cloud discovery: %w", err)
	}
	return bridges, nil
}
