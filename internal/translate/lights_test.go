package translate

import (
	"testing"

	"github.com/trick77/relume/internal/bridgepro"
)

func TestLightsV1_mapsAndAssignsStableIDs(t *testing.T) {
	// Given: two v2 lights (unsorted by UUID)
	var a, b bridgepro.Light
	a.ID = "bbbb-2"
	a.Metadata.Name = "Sofa"
	a.On.On = true
	a.Dimming.Brightness = 100
	a.Color.XY.X = 0.5
	a.Color.XY.Y = 0.4
	b.ID = "aaaa-1"
	b.Metadata.Name = "Decke"
	b.ColorTemperature.Mirek = 300

	// When: LightsV1 expects already sorted input (the client provides this);
	// here we simulate the sorted order aaaa, bbbb.
	lm := LightsV1([]bridgepro.Light{b, a})

	// Then: numeric IDs 1,2 stably point to the UUIDs
	if lm.V1ToUUID["1"] != "aaaa-1" || lm.V1ToUUID["2"] != "bbbb-2" {
		t.Fatalf("mapping wrong: %#v", lm.V1ToUUID)
	}
	light1 := lm.V1["1"].(map[string]any)
	if light1["name"] != "Decke" {
		t.Errorf("name = %v", light1["name"])
	}
	state1 := light1["state"].(map[string]any)
	if state1["colormode"] != "ct" || state1["ct"] != 300 {
		t.Errorf("ct state wrong: %#v", state1)
	}

	light2 := lm.V1["2"].(map[string]any)
	state2 := light2["state"].(map[string]any)
	if state2["colormode"] != "xy" {
		t.Errorf("colormode = %v, expected xy", state2["colormode"])
	}
	if state2["bri"] != 254 {
		t.Errorf("bri = %v, expected 254 (100%%)", state2["bri"])
	}
}

func TestBriFromPercent(t *testing.T) {
	cases := []struct {
		pct  float64
		want int
	}{{0, 1}, {100, 254}, {50, 127}}
	for _, c := range cases {
		if got := briFromPercent(c.pct); got != c.want {
			t.Errorf("briFromPercent(%v) = %d, want %d", c.pct, got, c.want)
		}
	}
}
