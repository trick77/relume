// Package bridge wires the TV-side frontend (clipv1) to the
// Bridge-Pro-side backend (bridgepro) and holds the light mapping.
package bridge

import (
	"fmt"
	"sync"
	"time"

	"github.com/trick77/relume/internal/bridgepro"
	"github.com/trick77/relume/internal/translate"
)

// lightCacheTTL limits how often the Bridge Pro is queried for lights.
const lightCacheTTL = 5 * time.Second

// LightProvider implements clipv1.LightProvider on top of the Bridge Pro and
// holds the v1→UUID mapping for later control (M3).
type LightProvider struct {
	client *bridgepro.Client

	mu        sync.Mutex
	cached    map[string]any
	v1ToUUID  map[string]string
	fetchedAt time.Time
}

// NewLightProvider creates a provider for the given Bridge Pro.
func NewLightProvider(client *bridgepro.Client) *LightProvider {
	return &LightProvider{client: client}
}

// LightsV1 returns the v1 light list (with a short cache).
func (p *LightProvider) LightsV1() (map[string]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cached != nil && time.Since(p.fetchedAt) < lightCacheTTL {
		return p.cached, nil
	}
	lights, err := p.client.Lights()
	if err != nil {
		return nil, err
	}
	lm := translate.LightsV1(lights)
	p.cached = lm.V1
	p.v1ToUUID = lm.V1ToUUID
	p.fetchedAt = time.Now()
	return p.cached, nil
}

// UUIDForV1 returns the v2 UUID for a numeric v1 light ID.
func (p *LightProvider) UUIDForV1(v1id string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	uuid, ok := p.v1ToUUID[v1id]
	return uuid, ok
}

// SetLightV1 sets the state of a light by its v1 ID. The v1 state is
// translated to v2 and forwarded to the Bridge Pro (REST fallback path).
func (p *LightProvider) SetLightV1(v1id string, v1state map[string]any) error {
	uuid, ok := p.UUIDForV1(v1id)
	if !ok {
		// Mapping may not be built yet → load lights once and check again.
		if _, err := p.LightsV1(); err != nil {
			return err
		}
		if uuid, ok = p.UUIDForV1(v1id); !ok {
			return fmt.Errorf("unknown light id %q", v1id)
		}
	}
	return p.client.SetLight(uuid, translate.StateV1ToV2(v1state))
}
