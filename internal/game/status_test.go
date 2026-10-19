package game

import (
	"testing"
	"time"
)

func TestPrepareStatus(t *testing.T) {
	now := time.Date(2026, time.September, 27, 9, 30, 0, 0, time.UTC)
	tests := []struct {
		name      string
		asset     *Asset
		wantQueue bool
	}{
		{name: "pending player asset enters moderation", asset: &Asset{ID: "skin-7", Kind: "skin", ModerationState: "pending"}, wantQueue: true},
		{name: "approved player asset stays live", asset: &Asset{ID: "map-2", Kind: "map", ModerationState: "approved"}, wantQueue: false},
		{name: "status without asset stays live", wantQueue: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := PrepareStatus(DeviceStatus{PlayerID: "player-42", DeviceID: "deck-3", State: "online", Asset: tt.asset}, now)
			if got := event.Status.Queue != nil; got != tt.wantQueue {
				t.Fatalf("queue set = %v, want %v", got, tt.wantQueue)
			}
			if event.Type != "device.status" || event.Status.At != "2026-09-27T09:30:00Z" {
				t.Fatalf("unexpected event: %#v", event)
			}
		})
	}
}
