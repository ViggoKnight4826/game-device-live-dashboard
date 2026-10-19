package game

import "time"

type Asset struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	ModerationState string `json:"moderation_state"`
}

type DeviceStatus struct {
	PlayerID string  `json:"player_id"`
	DeviceID string  `json:"device_id"`
	State    string  `json:"state"`
	Asset    *Asset  `json:"asset,omitempty"`
	At       string  `json:"at"`
	Queue    *string `json:"moderation_queue,omitempty"`
}

type LiveEvent struct {
	Type   string       `json:"type"`
	Status DeviceStatus `json:"status"`
}

func PrepareStatus(status DeviceStatus, now time.Time) LiveEvent {
	status.At = now.UTC().Format(time.RFC3339)
	if status.Asset != nil && status.Asset.ModerationState == "pending" {
		queue := "player-assets"
		status.Queue = &queue
	}
	return LiveEvent{Type: "device.status", Status: status}
}
