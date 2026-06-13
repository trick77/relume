package upnp

import (
	"strings"
	"testing"

	"github.com/trick77/relume/internal/config"
)

func TestRenderWithProfile_hassUsesHomeAssistantManufacturerFields(t *testing.T) {
	// Given
	id := config.Identity{Serial: "2c4d54ea2832"}

	// When
	xml, err := RenderWithProfile(id, "192.0.2.10", 80, "hass")

	// Then
	if err != nil {
		t.Fatalf("RenderWithProfile: %v", err)
	}
	for _, want := range []string{
		"<manufacturer>Royal Philips Electronics</manufacturer>",
		"<manufacturerURL>http://www.philips.com</manufacturerURL>",
		"<modelName>Philips hue bridge 2015</modelName>",
		"<modelNumber>BSB002</modelNumber>",
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("description.xml missing %q:\n%s", want, xml)
		}
	}
}
