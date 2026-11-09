package requests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Silo-Server/silo-server/internal/access"
)

// profileFetchTimeout bounds one download server's profile listing: the request
// dialog waits on it, so a server that does not answer loses its list rather
// than the whole dialog.
const profileFetchTimeout = 15 * time.Second

// configQualityProfileID is the Sonarr/Radarr plugin config key naming the
// quality profile a server downloads with.
const configQualityProfileID = "quality_profile_id"

// RequesterProfileChoices lists the quality profiles a requester may pick for a
// media type: every enabled download server that serves it, minus the profiles
// the admin's allowlist excludes and the 4K ones the requester's playback
// ceiling forbids. It reads each server's own profile list, so a profile added
// in Sonarr or Radarr (by profilarr or by hand) needs no change here.
func (s *Service) RequesterProfileChoices(ctx context.Context, viewer Viewer, mediaType MediaType) (ProfileChoices, error) {
	if err := validateViewer(viewer); err != nil {
		return ProfileChoices{}, err
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return ProfileChoices{}, err
	}
	if !settings.RequestsEnabled || !settings.RequesterQualityProfiles {
		return ProfileChoices{Servers: []ServerProfileChoices{}}, nil
	}
	fc, err := s.newFulfillContext(ctx)
	if err != nil {
		return ProfileChoices{}, err
	}
	ceiling, _ := s.requesterCeiling(ctx, viewer.UserID, viewer.ProfileID)
	servers := s.profileChoicesFor(ctx, fc, mediaType, settings.RequesterQualityProfileAllowlist, ceiling)
	return ProfileChoices{Enabled: len(servers) > 0, Servers: servers}, nil
}

// profileChoicesFor reads the profiles of each download server that serves the
// media type. A server that cannot be reached is logged and left out: the
// dialog still opens with the servers that answered.
func (s *Service) profileChoicesFor(ctx context.Context, fc *fulfillContext, mediaType MediaType, allowlist []string, ceiling string) []ServerProfileChoices {
	out := []ServerProfileChoices{}
	allowed4K := access.QualityAllowed(access.PlaybackQuality4K, ceiling)
	for _, in := range fc.integrations {
		if !eligibleRouterConnection(in, mediaType) {
			continue
		}
		if strings.TrimSpace(in.BaseURL) == "" || strings.TrimSpace(in.APIKeyRef) == "" {
			continue
		}
		profiles, err := listArrQualityProfiles(ctx, in.BaseURL, strings.TrimSpace(in.APIKeyRef))
		if err != nil {
			slog.WarnContext(ctx, "requests: cannot list quality profiles", "component", "requests",
				"connection_id", in.ID, "err", err)
			continue
		}
		defaultProfile := configInt(in.PluginConfig, configQualityProfileID)
		choices := make([]ProfileChoice, 0, len(profiles))
		for _, profile := range profiles {
			is4K := profile.is4K()
			if is4K && !allowed4K {
				// The requester cannot play 4K, so offering the profile would
				// only get them a copy they transcode.
				continue
			}
			if !profileAllowedByAllowlist(allowlist, in.ID, profile.ID) {
				continue
			}
			choices = append(choices, ProfileChoice{
				ID:      profile.ID,
				Name:    profile.Name,
				Is4K:    is4K,
				Default: defaultProfile != 0 && defaultProfile == profile.ID,
			})
		}
		if len(choices) == 0 {
			continue
		}
		out = append(out, ServerProfileChoices{ConnectionID: in.ID, ServerName: in.Name, Profiles: choices})
	}
	return out
}

// arrQualityProfile is the part of a Sonarr/Radarr quality profile the picker
// needs: its id, its name, and the qualities it allows.
type arrQualityProfile struct {
	ID    int                   `json:"id"`
	Name  string                `json:"name"`
	Items []arrQualityProfileIt `json:"items"`
}

type arrQualityProfileIt struct {
	Quality *struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		Resolution int    `json:"resolution"`
	} `json:"quality,omitempty"`
	Items   []arrQualityProfileIt `json:"items,omitempty"`
	Allowed bool                  `json:"allowed"`
}

// is4K reports whether the profile allows a 2160p quality, which is what makes
// picking it ask for a 4K copy as well as the HD one.
func (p arrQualityProfile) is4K() bool {
	var walk func(items []arrQualityProfileIt) bool
	walk = func(items []arrQualityProfileIt) bool {
		for _, item := range items {
			if item.Quality != nil && item.Allowed && item.Quality.Resolution >= 2160 {
				return true
			}
			if walk(item.Items) {
				return true
			}
		}
		return false
	}
	return walk(p.Items)
}

// listArrQualityProfiles reads a Sonarr/Radarr server's quality profiles from
// its own API. The request service speaks to the download server here rather
// than through the router plugin because the picker needs the profile's
// qualities, which the plugin's config options do not carry.
func listArrQualityProfiles(ctx context.Context, baseURL, apiKey string) ([]arrQualityProfile, error) {
	url := strings.TrimRight(baseURL, "/") + "/api/v3/qualityprofile"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: profileFetchTimeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("quality profiles: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var profiles []arrQualityProfile
	if err := json.NewDecoder(resp.Body).Decode(&profiles); err != nil {
		return nil, fmt.Errorf("quality profiles: decode: %w", err)
	}
	return profiles, nil
}

// profileAllowedByAllowlist reports whether the admin's allowlist lets this
// profile be offered. An empty allowlist allows every profile.
func profileAllowedByAllowlist(allowlist []string, connectionID string, profileID int) bool {
	if len(allowlist) == 0 {
		return true
	}
	want := connectionID + ":" + strconv.Itoa(profileID)
	for _, entry := range allowlist {
		if strings.TrimSpace(entry) == want {
			return true
		}
	}
	return false
}

// resolveRequesterQualityProfile checks the profile a requester picked against
// what the server currently offers, and fills in the name, the 4K tier and the
// connection so the row carries them. A pick that is no longer offered (the
// admin turned the feature off, the profile was deleted in Sonarr or Radarr, or
// the requester may not have 4K) is refused rather than silently dropped, so
// the requester is not left with a different download than they asked for.
func (s *Service) resolveRequesterQualityProfile(ctx context.Context, viewer Viewer, input *CreateRequestInput) error {
	if input.QualityProfileID == nil {
		return nil
	}
	profileID := *input.QualityProfileID
	if profileID <= 0 {
		return &ValidationError{FieldErrors: map[string]string{fieldQualityProfileID: "Pick a quality profile from the list."}}
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.RequesterQualityProfiles {
		return &ValidationError{FieldErrors: map[string]string{fieldQualityProfileID: "Choosing a quality profile is turned off."}}
	}
	fc, err := s.newFulfillContext(ctx)
	if err != nil {
		return err
	}
	ceiling, _ := s.requesterCeiling(ctx, viewer.UserID, viewer.ProfileID)
	servers := s.profileChoicesFor(ctx, fc, input.MediaType, settings.RequesterQualityProfileAllowlist, ceiling)
	for _, server := range servers {
		if input.QualityProfileConnectionID != "" && server.ConnectionID != input.QualityProfileConnectionID {
			continue
		}
		for _, profile := range server.Profiles {
			if profile.ID != profileID {
				continue
			}
			input.QualityProfileName = profile.Name
			input.QualityProfile4K = profile.Is4K
			input.QualityProfileConnectionID = server.ConnectionID
			return nil
		}
	}
	return &ValidationError{FieldErrors: map[string]string{fieldQualityProfileID: "That quality profile is not available for this request."}}
}

// fieldQualityProfileID names the quality profile in a validation error.
const fieldQualityProfileID = "quality_profile_id"

// applyRequesterProfile replaces the quality profile of the connection a tier
// is sent to with the one the requester picked, when the pick applies to that
// tier: a 4K profile governs the 4K copy only, and any other profile the HD
// copy only, so a 4K pick still downloads the 1080 copy the server's own
// settings describe.
func applyRequesterProfile(req Request, conns []ResolvedRouterConnection, tier Quality) []ResolvedRouterConnection {
	if req.QualityProfileID == nil || len(conns) == 0 {
		return conns
	}
	want := Quality1080p
	if req.QualityProfile4K {
		want = Quality2160p
	}
	if tier != want {
		return conns
	}
	// The pick was made on one server's list. Send it there and leave a
	// sibling server's own profile alone.
	target := req.QualityProfileConnectionID
	out := make([]ResolvedRouterConnection, 0, len(conns))
	applied := false
	for _, conn := range conns {
		next := conn
		if target != "" && conn.ID == target {
			next.Config = withQualityProfile(conn.Config, *req.QualityProfileID)
			applied = true
		}
		out = append(out, next)
	}
	if applied {
		return out
	}
	// The route sent the tier to a server the pick did not name (or the row
	// predates the connection field): apply it to the connection that serves
	// the tier.
	for i, conn := range out {
		if boolConfig(conn.Config, tierDefaultConfigKey(tier)) {
			out[i].Config = withQualityProfile(conn.Config, *req.QualityProfileID)
			return out
		}
	}
	return out
}

// tierDefaultConfigKey names the plugin config key marking the connection a
// tier defaults to.
func tierDefaultConfigKey(tier Quality) string {
	if tier == Quality2160p {
		return configIsDefault4K
	}
	return configIsDefault
}

// withQualityProfile returns a copy of a plugin config carrying profileID. The
// plugin config arrives from JSON, so numbers are float64.
func withQualityProfile(config map[string]any, profileID int) map[string]any {
	out := make(map[string]any, len(config)+1)
	for key, value := range config {
		out[key] = value
	}
	out[configQualityProfileID] = float64(profileID)
	return out
}

// configInt reads an integer plugin config value, which may be stored as a
// number or a string.
func configInt(config map[string]any, key string) int {
	value, ok := config[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}
