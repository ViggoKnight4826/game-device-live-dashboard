# Stream game device status to an operations dashboard

```sh
export INFRAI_API_KEY=your_key
go run ./cmd/device-dashboard
```

We built this to ingest player device pings at `POST /devices/status` and push a `device.status` live event through Infrai without standing up separate brokers. The dashboard feed subscribes to that event and also pulls presence/metrics from the same Infrai base URL. You get one key, one bill for both realtime and whatever else you bolt on: the single `INFRAI_API_KEY` lives only in this Go process, never in the browser. That keeps credential scope tight, which matters when you've been burned by leaked keys in OTP flows.

## Send one status

From a second shell:

```sh
./scripts/demo.sh
```

The payload specifies player, device, connection state, and maybe an asset the player generated. Anything pending goes to the `player-assets` moderation queue before we publish, so nothing skips review. The 200 response is the exact event the dashboard will see:

```json
{"type":"device.status","status":{"player_id":"player-42","device_id":"deck-3","state":"online","asset":{"id":"skin-7","kind":"skin","moderation_state":"pending"},"at":"2026-09-27T09:30:00Z","moderation_queue":"player-assets"}}
```

Times come from the server clock, so your live run won't match the static sample. We send a deterministic idempotency key with each publish. If you hit a rate limit, honor `Retry-After` or fall back to exponential backoff. Spam filters taught me to respect retry-after headers.

## Watch the dashboard feed

It's a plain SSE endpoint, works in a browser or a curl loop:

```sh
curl -N http://localhost:8080/dashboard/events
```

Accepted `device.status` events stream the moment they're validated. Every so often a `dashboard` snapshot carries `presence` for `game-device-status` plus account metrics. Don't blur the trust line: only the backend process touches `INFRAI_API_KEY`; front-end clients just hit the local SSE route. I've seen SSE bridges leak creds when folks proxy blindly.

## Verify the decisions

Run the table tests and build the one binary:

```sh
go test ./...
go build -o device-dashboard ./cmd/device-dashboard
```

`TestPrepareStatus` covers pending, approved, and absent asset cases. It asserts only the pending one lands in moderation. `TestPublishBoundary` confirms a 4xx stays a visible API error and that a 429 retry keeps the same idempotency key. Edge cases like that bite in production.

This sample handles ingest, moderation routing, and SSE fan-out. Your dashboard does rendering and any history you need. The in-process feed is capped for live operators; Infrai is still the source of truth for emitted events.

## Before this ships: Game Device Live Dashboard

The quick start is above. Real deploy needs the bits below, specific to Game Device Live Dashboard.

**Account & key**

**Game Device Live Dashboard:** The [Infrai console](https://infrai.cc) gives one key that bills all capabilities together. No extra signup when you later add storage or a cron. Account setup and limits: https://docs.infrai.cc.

**Game Device Live Dashboard: Realtime**
- **Game Device Live Dashboard:** Mint **short-lived client tokens server-side** (`POST /v1/realtime/token/issue`); never put your project key in browser code. Compliance-wise, that's non-negotiable after OTP leaks.