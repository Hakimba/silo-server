package requests

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
)

// Quality profiles are read from the request router plugin rather than from
// Sonarr or Radarr: in a Seerr-fronted install the host holds no arr
// credentials, and Seerr is the only one who can list a server's profiles.
// The plugin publishes them as config options, the same way it fills the admin
// form's profile selects, so the host needs no per-backend knowledge.

// profileOptionField is the config field a router plugin publishes its quality
// profiles under. The Seerr plugin also publishes per-kind fields, which this
// host reads when a media type's server entry is a Seerr one.
const (
	profileOptionFieldGeneric = "quality_profile_id"
	profileOptionFieldRadarr  = "radarr_quality_profile_id"
	profileOptionFieldSonarr  = "sonarr_quality_profile_id"
	// profileOptionValueDefault is the option meaning "no choice": the server's
	// own profile. It is offered so the dialog can name the default, and a
	// request that picks it stores no profile.
	profileOptionValueDefault = "default"
)

// profileOptionFields lists the option fields to try for a media type, in
// order.
func profileOptionFields(mediaType MediaType) []string {
	if mediaType == MediaTypeSeries {
		return []string{profileOptionFieldSonarr, profileOptionFieldGeneric}
	}
	return []string{profileOptionFieldRadarr, profileOptionFieldGeneric}
}

// RequesterProfileChoices lists the quality profiles a requester may pick for a
// media type, asking every enabled download server that serves it. A server
// the plugin cannot read is logged and left out: the dialog still opens with
// the servers that answered.
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
	if s.router == nil {
		return ProfileChoices{Servers: []ServerProfileChoices{}}, nil
	}
	fc, err := s.newFulfillContext(ctx)
	if err != nil {
		return ProfileChoices{}, err
	}
	servers := s.profileChoicesFor(ctx, fc, mediaType, settings.RequesterQualityProfileAllowlist)
	return ProfileChoices{Enabled: len(servers) > 0, Servers: servers}, nil
}

// profileChoicesFor asks each serving connection's plugin for its profiles.
func (s *Service) profileChoicesFor(ctx context.Context, fc *fulfillContext, mediaType MediaType, allowlist []string) []ServerProfileChoices {
	out := []ServerProfileChoices{}
	for _, in := range fc.integrations {
		if !eligibleRouterConnection(in, mediaType) || in.InstallationID == nil {
			continue
		}
		missing, err := s.missingAPIKey(ctx, fc, in)
		if err != nil || missing {
			continue
		}
		conn := ResolvedRouterConnection{ID: in.ID, BaseURL: in.BaseURL, APIKey: strings.TrimSpace(in.APIKeyRef), Config: in.PluginConfig}
		options, err := s.router.ListConfigOptions(ctx, *in.InstallationID, in.CapabilityID, conn)
		if err != nil {
			slog.WarnContext(ctx, "requests: cannot list quality profiles", "component", "requests",
				"connection_id", in.ID, "err", err)
			continue
		}
		choices := profileChoicesFrom(options, profileOptionFields(mediaType), in.ID, allowlist)
		if len(choices) == 0 {
			continue
		}
		out = append(out, ServerProfileChoices{ConnectionID: in.ID, ServerName: in.Name, Profiles: choices})
	}
	return out
}

// profileChoicesFrom turns one plugin's option map into the picker's list: the
// first field that answered wins, its "default" option (when the plugin offers
// one) becomes choice 0, and the admin's allowlist is applied.
func profileChoicesFrom(options map[string][]RouterOption, fields []string, connectionID string, allowlist []string) []ProfileChoice {
	for _, field := range fields {
		list := options[field]
		if len(list) == 0 {
			continue
		}
		out := make([]ProfileChoice, 0, len(list))
		for _, option := range list {
			value := strings.TrimSpace(option.Value)
			name := strings.TrimSpace(option.Label)
			if strings.EqualFold(value, profileOptionValueDefault) {
				out = append(out, ProfileChoice{ID: 0, Name: name, Default: true})
				continue
			}
			id, err := strconv.Atoi(value)
			if err != nil || id <= 0 {
				continue
			}
			if !profileAllowedByAllowlist(allowlist, connectionID, id) {
				continue
			}
			out = append(out, ProfileChoice{ID: id, Name: name})
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
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
// what the server currently offers, and fills in the name and the connection so
// the row carries them. A pick that is no longer offered (the admin turned the
// feature off, the profile was deleted in Radarr or Sonarr) is refused rather
// than silently dropped, so the requester is not left with a different download
// than they asked for. A pick of the "server default" option stores nothing.
func (s *Service) resolveRequesterQualityProfile(ctx context.Context, viewer Viewer, input *CreateRequestInput) error {
	if input.QualityProfileID == nil {
		return nil
	}
	profileID := *input.QualityProfileID
	if profileID < 0 {
		return &ValidationError{FieldErrors: map[string]string{fieldQualityProfileID: "Pick a quality profile from the list."}}
	}
	if profileID == 0 {
		input.QualityProfileID = nil
		return nil
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
	for _, server := range s.profileChoicesFor(ctx, fc, input.MediaType, settings.RequesterQualityProfileAllowlist) {
		if input.QualityProfileConnectionID != "" && server.ConnectionID != input.QualityProfileConnectionID {
			continue
		}
		for _, profile := range server.Profiles {
			if profile.ID != profileID {
				continue
			}
			input.QualityProfileName = profile.Name
			input.QualityProfileConnectionID = server.ConnectionID
			return nil
		}
	}
	return &ValidationError{FieldErrors: map[string]string{fieldQualityProfileID: "That quality profile is not available for this request."}}
}

// fieldQualityProfileID names the quality profile in a validation error.
const fieldQualityProfileID = "quality_profile_id"

// applyRequesterProfile replaces the quality profile of the connection the
// request is sent to with the one the requester picked. The pick was made on
// one server's list, so it is sent there and a sibling server's own profile is
// left alone.
func applyRequesterProfile(req Request, conns []ResolvedRouterConnection) []ResolvedRouterConnection {
	if req.QualityProfileID == nil || len(conns) == 0 {
		return conns
	}
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
	// The row predates the connection field: apply the pick to every server the
	// request is being sent to.
	for i := range out {
		out[i].Config = withQualityProfile(out[i].Config, *req.QualityProfileID)
	}
	return out
}

// withQualityProfile returns a copy of a plugin config carrying profileID. The
// config arrives from JSON, so numbers are float64.
func withQualityProfile(config map[string]any, profileID int) map[string]any {
	out := make(map[string]any, len(config)+1)
	out[profileOptionFieldGeneric] = float64(profileID)
	for key, value := range config {
		out[key] = value
	}
	return out
}
