package bridgepro

// SetLight sets the state of a light (CLIP v2 PUT on the light resource).
func (c *Client) SetLight(uuid string, v2body map[string]any) error {
	return c.put("/clip/v2/resource/light/"+uuid, v2body)
}

// SetGroupedLight sets the state of a grouped_light (room/zone group control).
func (c *Client) SetGroupedLight(uuid string, v2body map[string]any) error {
	return c.put("/clip/v2/resource/grouped_light/"+uuid, v2body)
}
