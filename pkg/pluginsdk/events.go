package pluginsdk

import "encoding/json"

// EventSubscribeParams subscribes to a core event channel.
type EventSubscribeParams struct {
	Channel string `json:"channel"`
}

// EventPublishParams publishes a plugin-scoped event.
type EventPublishParams struct {
	Channel string          `json:"channel"`
	Payload json.RawMessage `json:"payload"`
}

// Subscribe requests a core event channel subscription.
func (c *Client) Subscribe(channel string) error {
	return c.CallCore("events.subscribe", EventSubscribeParams{Channel: channel}, nil)
}

// Publish emits an event on a plugin-owned channel.
func (c *Client) Publish(channel string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.CallCore("events.publish", EventPublishParams{
		Channel: channel,
		Payload: raw,
	}, nil)
}
