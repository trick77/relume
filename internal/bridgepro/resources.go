package bridgepro

import (
	"sort"
)

// Light is the subset of a CLIP v2 light resource that is relevant for relume.
type Light struct {
	ID       string `json:"id"`
	IDv1     string `json:"id_v1"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	On struct {
		On bool `json:"on"`
	} `json:"on"`
	Dimming struct {
		Brightness float64 `json:"brightness"`
	} `json:"dimming"`
	Color struct {
		XY struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"xy"`
	} `json:"color"`
	ColorTemperature struct {
		Mirek int `json:"mirek"`
	} `json:"color_temperature"`
	// Owner references the associated device (for stable sorting/names).
	Owner struct {
		RID string `json:"rid"`
	} `json:"owner"`
}

type lightList struct {
	Errors []any   `json:"errors"`
	Data   []Light `json:"data"`
}

// Lights reads all lights of the Bridge Pro, stably sorted by ID.
func (c *Client) Lights() ([]Light, error) {
	var ll lightList
	if err := c.get("/clip/v2/resource/light", &ll); err != nil {
		return nil, err
	}
	sort.Slice(ll.Data, func(i, j int) bool { return ll.Data[i].ID < ll.Data[j].ID })
	return ll.Data, nil
}

// EntertainmentConfig is the subset of an entertainment_configuration resource
// that is relevant for relume.
type EntertainmentConfig struct {
	ID       string `json:"id"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status string `json:"status"` // "inactive" | "active"
}

type entConfigList struct {
	Errors []any                 `json:"errors"`
	Data   []EntertainmentConfig `json:"data"`
}

// EntertainmentConfigs reads the entertainment configurations of the Bridge Pro.
func (c *Client) EntertainmentConfigs() ([]EntertainmentConfig, error) {
	var el entConfigList
	if err := c.get("/clip/v2/resource/entertainment_configuration", &el); err != nil {
		return nil, err
	}
	sort.Slice(el.Data, func(i, j int) bool { return el.Data[i].ID < el.Data[j].ID })
	return el.Data, nil
}
