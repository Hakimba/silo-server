-- +goose Up
-- +goose StatementBegin
-- A requester may pick one of the quality profiles configured on the Sonarr or
-- Radarr server their request is sent to. The choice is stored on the request
-- (id, name, whether it is a 4K profile, and the connection it came from) so
-- the submission can send that tier with it and the API can show what was
-- asked for.
ALTER TABLE public.media_requests
    ADD COLUMN IF NOT EXISTS quality_profile_id integer,
    ADD COLUMN IF NOT EXISTS quality_profile_name text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS quality_profile_4k boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS quality_profile_connection_id text NOT NULL DEFAULT '';

ALTER TABLE public.request_settings
    ADD COLUMN IF NOT EXISTS requester_quality_profiles boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS requester_quality_profile_allowlist text[] NOT NULL DEFAULT '{}';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE public.request_settings
    DROP COLUMN IF EXISTS requester_quality_profiles,
    DROP COLUMN IF EXISTS requester_quality_profile_allowlist;

ALTER TABLE public.media_requests
    DROP COLUMN IF EXISTS quality_profile_id,
    DROP COLUMN IF EXISTS quality_profile_name,
    DROP COLUMN IF EXISTS quality_profile_4k,
    DROP COLUMN IF EXISTS quality_profile_connection_id;
-- +goose StatementEnd
