# Requester-chosen quality profiles

A requester can pick which quality profile their request downloads with, the
way Seerr and Moonfin do. In a Seerr-fronted install the list comes from Seerr,
which is the only one who can read a server's profiles — Silo holds no Radarr or
Sonarr credentials there.

## What it does

- The request dialog offers the profiles Seerr publishes for the server the
  request will go to: Radarr's for a movie, Sonarr's for a series.
- Picking one sends `profileId` with the request, so Seerr uses that profile
  **for that request only** and leaves the server entry's own profile alone.
- Picking the "server default" entry sends no profile at all: exactly what
  happens today.
- A request that picks nothing is unchanged, and the server's own default
  applies.
- A profile deleted in Radarr or Sonarr stops being offered; a request that
  still names it is refused with a field error rather than silently downloading
  something else.

One copy per title: a single Radarr instance holds one file per movie, so this
chooses *which* profile that copy downloads with, not how many copies there are.
Two copies (1080 + 4K) need a second arr instance and a second Seerr server
entry — out of scope here.

## Where the profiles come from

The host asks the request router plugin, which asks Seerr
(`GET /api/v1/service/radarr/{id}` and the Sonarr equivalent). The plugin
publishes them as config options, the same list that fills the admin form's
profile selects, so the host needs no per-backend knowledge.

The Seerr plugin publishes them per kind (`radarr_quality_profile_id`,
`sonarr_quality_profile_id`, plus the 4K variants when a 4K server is
configured); the arr plugin keeps publishing `quality_profile_id`. The host
reads whichever the media type maps to, and always writes `quality_profile_id`
on the connection it hands to the plugin at dispatch — the key both plugins
read as "the profile for this request".

## Turning it on

Settings → Requests → **Let people choose a quality profile**, off by default.
The API also accepts an allowlist (`requester_quality_profile_allowlist`, each
entry `connection-id:profile-id`); empty offers every profile the plugin
publishes.

## Data and API

- `media_requests` gains `quality_profile_id`, `quality_profile_name` and
  `quality_profile_connection_id`; the name is kept so a request still shows
  what was asked for after the profile is renamed.
- `request_settings` gains `requester_quality_profiles` and
  `requester_quality_profile_allowlist`.
- New: `GET /api/v2/requests/profile-options?media_type=movie|series` returns
  `{ enabled, servers: [{ connection_id, server_name, profiles: [{ id, name,
  default }] }] }`, classed like the other requester operations.
- `POST /api/v2/requests` accepts `quality_profile_id` and
  `quality_profile_connection_id` (the latter only needed when two servers
  publish the same profile id). Id `0` is the server's own default.

## Running it

The plugin must be updated first: the fork's branch adds the profile option
lists and `profileId` on the Seerr request. Build it and upload it in Admin →
Plugins → Upload, replacing the installed `silo.requests.seerr`:

```sh
cd silo-plugins-requests-seerr
go build -trimpath -o plugin-linux-amd64 .
```

Then build the server image from this branch and swap it in:

```sh
docker build -t silo:quality-profiles .
docker build -f omnibus/Dockerfile --build-arg SILO_IMAGE=silo:quality-profiles -t silo-omnibus:quality-profiles .
```

The migration only adds columns, and Silo takes its usual backup before
startup migrations.

## Tests

- Plugin (`go test ./...`): the profile options list the targeted server's
  profiles and name Seerr's default; the configured profile is sent as
  `profileId`; a requester's pick overrides it; nothing is sent without one; a
  profile the server does not offer fails the target without asking Seerr.
- Server (`go test ./internal/requests/`): the picker asks the plugin and maps
  its options (including the default entry), the admin switch and allowlist are
  honoured, an unreachable plugin leaves the server out, the pick is validated
  and stored with its name and connection, `0` stores nothing, and the dispatch
  config carries the profile only for the picked connection.
- Web (`pnpm test`): the picker component and the season dialog sending the
  pick.
