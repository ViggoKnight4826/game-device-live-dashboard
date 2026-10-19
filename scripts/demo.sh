#!/bin/sh
set -eu

curl --fail-with-body --silent --show-error \
  -X POST http://localhost:8080/devices/status \
  -H 'Content-Type: application/json' \
  -d '{"player_id":"player-42","device_id":"deck-3","state":"online","asset":{"id":"skin-7","kind":"skin","moderation_state":"pending"}}'
