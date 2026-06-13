// Package upnp renders the /description.xml that the TV fetches after SSDP
// discovery and checks for modelName/modelNumber in order to recognize the
// bridge as a Gen-2 Hue bridge.
package upnp

import (
	"strings"
	"text/template"

	"github.com/trick77/relume/internal/config"
)

type profileFields struct {
	Manufacturer    string
	ManufacturerURL string
}

func fieldsForProfile(profile string) profileFields {
	if profile == "hass" {
		return profileFields{
			Manufacturer:    "Royal Philips Electronics",
			ManufacturerURL: "http://www.philips.com",
		}
	}
	return profileFields{
		Manufacturer:    "Signify",
		ManufacturerURL: "http://www.meethue.com",
	}
}

// modelName/modelNumber are exactly the values of a Philips Hue Bridge 2015 (BSB002);
// the TV only recognizes these as compatible.
const tmplText = `<?xml version="1.0" encoding="UTF-8" ?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
<specVersion>
<major>1</major>
<minor>0</minor>
</specVersion>
<URLBase>http://{{.IP}}:{{.Port}}/</URLBase>
<device>
<deviceType>urn:schemas-upnp-org:device:Basic:1</deviceType>
<friendlyName>Philips hue ({{.IP}})</friendlyName>
<manufacturer>{{.Manufacturer}}</manufacturer>
<manufacturerURL>{{.ManufacturerURL}}</manufacturerURL>
<modelDescription>Philips hue Personal Wireless Lighting</modelDescription>
<modelName>Philips hue bridge 2015</modelName>
<modelNumber>BSB002</modelNumber>
<modelURL>http://www.meethue.com</modelURL>
<serialNumber>{{.Serial}}</serialNumber>
<UDN>uuid:{{.UUID}}</UDN>
<presentationURL>index.html</presentationURL>
</device>
</root>
`

var tmpl = template.Must(template.New("description").Parse(tmplText))

// Render generates the description.xml for the given identity and address.
func Render(id config.Identity, ip string, port int) (string, error) {
	return RenderWithProfile(id, ip, port, "")
}

// RenderWithProfile generates description.xml for an identity profile. The empty
// profile keeps relume's default bridge identity; "hass" matches Home Assistant
// emulated-hue fields that public Philips TV reports have accepted.
func RenderWithProfile(id config.Identity, ip string, port int, profile string) (string, error) {
	var sb strings.Builder
	fields := fieldsForProfile(profile)
	err := tmpl.Execute(&sb, struct {
		IP              string
		Port            int
		Serial          string
		UUID            string
		Manufacturer    string
		ManufacturerURL string
	}{
		IP:              ip,
		Port:            port,
		Serial:          id.Serial,
		UUID:            id.UUID(),
		Manufacturer:    fields.Manufacturer,
		ManufacturerURL: fields.ManufacturerURL,
	})
	return sb.String(), err
}
