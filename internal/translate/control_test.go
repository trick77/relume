package translate

import (
	"reflect"
	"testing"
)

func TestStateV1ToV2(t *testing.T) {
	// Given: a typical Ambilight v1 state with on/bri/xy/transitiontime
	v1 := map[string]any{
		"on":             true,
		"bri":            float64(254),
		"xy":             []any{0.3, 0.4},
		"transitiontime": float64(1),
	}

	// When
	v2 := StateV1ToV2(v1)

	// Then
	if !reflect.DeepEqual(v2["on"], map[string]any{"on": true}) {
		t.Errorf("on = %#v", v2["on"])
	}
	dim := v2["dimming"].(map[string]any)
	if dim["brightness"].(float64) != 100 {
		t.Errorf("brightness = %v, expected 100", dim["brightness"])
	}
	col := v2["color"].(map[string]any)["xy"].(map[string]any)
	if col["x"] != 0.3 || col["y"] != 0.4 {
		t.Errorf("xy = %#v", col)
	}
	dyn := v2["dynamics"].(map[string]any)
	if dyn["duration"] != 100 {
		t.Errorf("duration = %v, expected 100ms", dyn["duration"])
	}
}

func TestStateV1ToV2_xyFloat64Slice(t *testing.T) {
	// Given: the entertainment decode path (entertainment.ToHueV1State) produces
	// xy as a []float64, not the []any that a JSON-decoded TV REST PUT yields.
	// StateV1ToV2 must still emit the color, otherwise the Pro only gets on/bri
	// and the lights keep their last colour (observed as "stuck red").
	v1 := map[string]any{
		"on":  true,
		"bri": 200,
		"xy":  []float64{0.2, 0.6},
	}

	// When
	v2 := StateV1ToV2(v1)

	// Then
	colWrap, ok := v2["color"].(map[string]any)
	if !ok {
		t.Fatalf("color missing for []float64 xy: %#v", v2)
	}
	col := colWrap["xy"].(map[string]any)
	if col["x"] != 0.2 || col["y"] != 0.6 {
		t.Errorf("xy = %#v", col)
	}
}

func TestStateV1ToV2_briInt64(t *testing.T) {
	// Given: bri arrives as an int64 (the "stuck red" bug class — an unhandled
	// numeric width silently dropped the value).
	v2 := StateV1ToV2(map[string]any{"on": true, "bri": int64(254)})

	// Then: dimming must still be emitted at full brightness.
	dim, ok := v2["dimming"].(map[string]any)
	if !ok {
		t.Fatalf("dimming missing for int64 bri: %#v", v2)
	}
	if dim["brightness"].(float64) != 100 {
		t.Errorf("brightness = %v, expected 100", dim["brightness"])
	}
}

func TestStateV1ToV2_malformedXYDropped(t *testing.T) {
	// Given: an xy pair whose components are not numeric. Emitting color anyway
	// would push a bogus (0,0) to the Pro; the pair must be dropped instead.
	v2 := StateV1ToV2(map[string]any{"on": true, "xy": []any{"oops", 0.4}})

	// Then
	if _, hasColor := v2["color"]; hasColor {
		t.Errorf("color should be dropped for malformed xy: %#v", v2["color"])
	}
}

func TestStateV1ToV2_ctOnly(t *testing.T) {
	// Given: only white/CT control
	v2 := StateV1ToV2(map[string]any{"on": false, "ct": float64(300)})

	// Then
	if _, hasColor := v2["color"]; hasColor {
		t.Error("color should not be set")
	}
	if v2["color_temperature"].(map[string]any)["mirek"] != 300 {
		t.Errorf("mirek wrong: %#v", v2["color_temperature"])
	}
}
