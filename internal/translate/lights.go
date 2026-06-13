// Package translate translates between the CLIP v2 model of the Bridge Pro and the
// CLIP v1 representation that the Ambilight TV expects, including a stable
// mapping between v1 light IDs (numeric) and v2 resource UUIDs.
package translate

import (
	"strconv"

	"github.com/trick77/relume/internal/bridgepro"
)

// LightMap holds the v1 representation of the lights and the ID mapping for control.
type LightMap struct {
	// V1 is the CLIP v1 light list (key = numeric ID as string).
	V1 map[string]any
	// V1ToUUID maps the numeric v1 ID to the v2 resource UUID.
	V1ToUUID map[string]string
}

// LightsV1 translates the v2 lights into the v1 structure. The Bridge Pro no longer
// provides reliable id_v1 values (CLIP v2); therefore we assign stable numeric
// IDs based on the UUID-sorted order.
func LightsV1(lights []bridgepro.Light) LightMap {
	v1 := make(map[string]any, len(lights))
	rev := make(map[string]string, len(lights))
	for i, l := range lights {
		id := strconv.Itoa(i + 1)
		rev[id] = l.ID
		v1[id] = lightV1(l)
	}
	return LightMap{V1: v1, V1ToUUID: rev}
}

// lightV1 builds a single v1 light object from a v2 light.
func lightV1(l bridgepro.Light) map[string]any {
	state := map[string]any{
		"on":        l.On.On,
		"bri":       briFromPercent(l.Dimming.Brightness),
		"alert":     "none",
		"reachable": true,
	}
	// Color/white mode: v2 uses xy or mirek.
	if l.Color.XY.X != 0 || l.Color.XY.Y != 0 {
		state["xy"] = []float64{l.Color.XY.X, l.Color.XY.Y}
		state["colormode"] = "xy"
	}
	if l.ColorTemperature.Mirek != 0 {
		state["ct"] = l.ColorTemperature.Mirek
		if _, hasXY := state["xy"]; !hasXY {
			state["colormode"] = "ct"
		}
	}
	name := l.Metadata.Name
	if name == "" {
		name = "Hue light " + l.ID[:8]
	}
	return map[string]any{
		"state":            state,
		"type":             "Extended color light",
		"name":             name,
		"modelid":          "LCT015",
		"manufacturername": "Signify Netherlands B.V.",
		"productname":      "Hue color lamp",
		"uniqueid":         l.ID,
		"swversion":        "1.122.2",
	}
}

// briFromPercent converts the v2 brightness (0..100 %) into v1 bri (1..254).
func briFromPercent(pct float64) int {
	if pct <= 0 {
		return 1
	}
	bri := int(pct/100.0*253.0) + 1
	if bri > 254 {
		return 254
	}
	return bri
}
