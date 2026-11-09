# Requester-chosen quality profiles

A requester can pick which quality profile their request downloads with, the
way Seerr and Moonfin do. The server reads the profiles straight from Radarr
and Sonarr, so profiles added by profilarr or the TRaSH guides appear without
any change here.

## What it does

- The request dialog offers every profile the download server that serves the
  media type publishes: Radarr's for a movie, Sonarr's for a series.
- A profile that allows a 2160p quality counts as a **4K profile**. Picking one
  asks for **two copies**: the 4K one with the chosen profile, and the 1080 one
  with the server's own 1080 profile, so screens without 4K play the 1080 copy
  and never transcode.
- Picking any other profile keeps the download **HD only**, even on a server
  that hands 4K to everyone.
- 4K profiles are offered only to profiles whose playback ceiling allows 4K
  (Settings → Users / Access groups), so a member without a 4K screen never sees
  them.
- A request that picks nothing behaves exactly as before: the server's own
  profile, and the server's own 1080/4K policy.

## Turning it on

Settings → Requests → **Let people choose a quality profile**, off by default.
The API also accepts an allowlist (`requester_quality_profile_allowlist`, each
entry `connection-id:profile-id`); empty offers everything the requester's
playback ceiling allows.

## Data and API

- `media_requests` gains `quality_profile_id`, `quality_profile_name`,
  `quality_profile_4k` and `quality_profile_connection_id`; the name is kept so
  a request still shows what was asked for after the profile is renamed.
- `request_settings` gains `requester_quality_profiles` and
  `requester_quality_profile_allowlist`.
- New: `GET /api/v2/requests/profile-options?media_type=movie|series` returns
  `{ enabled, servers: [{ connection_id, server_name, profiles: [{ id, name,
  is_4k, default }] }] }`, classed like the other requester operations.
- `POST /api/v2/requests` accepts `quality_profile_id` and
  `quality_profile_connection_id` (the latter only needed when two servers
  publish the same profile id).

The pick is merged into the connection config at dispatch, on the tier it
governs — the router plugin needs no change, since `quality_profile_id` is
already a key of that config.

## Before testing on a real install

- A 4K copy needs somewhere to go: the media type's routing must have a 4K
  destination (the Radarr connection marked 4K, or an UHD route destination).
  Without one the 4K tier is dropped, exactly as it is today without a picker.
- The migration only adds columns; Silo takes its usual backup before startup
  migrations.
- A profile deleted in Radarr while the dialog is open is refused with a field
  error rather than silently replaced.

## Tests

- `go test ./internal/requests/`: profile parsing (a TRaSH-shaped profile with
  2160p disallowed is not 4K), the HTTP read of `/api/v3/qualityprofile`, the
  allowlist, the tier the pick is applied to, the quality policy (a 1080 pick
  never gets a 4K copy), and the listing endpoint against a fake Radarr.
- `pnpm test`: the picker component and the season dialog sending the pick.

## Trying it on a real install

The branch is `feature/requester-quality-profile` on the fork (mirrored to
`apiv2`, the branch name the image pipeline builds from).

Either pull the image the fork publishes after a green CI:

```sh
docker login ghcr.io -u <github-user>        # the package starts private
docker pull ghcr.io/<github-user>/silo-server:<commit-sha>
```

and point `SILO_OMNIBUS_IMAGE` (or the compose `image:`) at it, or build it on
the server from the branch:

```sh
git fetch origin feature/requester-quality-profile
git checkout feature/requester-quality-profile
docker build -t silo:quality-profiles .
```

Then, in Settings → Requests, turn on **Let people choose a quality profile**
and open a movie: the Request button now asks for the profile first.
