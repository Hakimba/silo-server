package requests

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// trashProfile is a quality profile shaped like the ones profilarr writes into
// Radarr from the TRaSH guides: nested groups of allowed qualities.
const trashProfileJSON = `[
  {
    "id": 3,
    "name": "French HD 1080p",
    "items": [
      {"quality": {"id": 7, "name": "Bluray-1080p", "resolution": 1080}, "allowed": true},
      {"quality": {"id": 4, "name": "HDTV-1080p", "resolution": 1080}, "allowed": true},
      {"items": [
         {"quality": {"id": 19, "name": "Bluray-2160p", "resolution": 2160}, "allowed": false}
      ], "allowed": true}
    ]
  },
  {
    "id": 4,
    "name": "French 4K Remux",
    "items": [
      {"quality": {"id": 19, "name": "Bluray-2160p", "resolution": 2160}, "allowed": true},
      {"quality": {"id": 7, "name": "Bluray-1080p", "resolution": 1080}, "allowed": true}
    ]
  }
]`

func TestArrQualityProfileIs4K(t *testing.T) {
	var profiles []arrQualityProfile
	if err := json.Unmarshal([]byte(trashProfileJSON), &profiles); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("profiles = %d, want 2", len(profiles))
	}
	if profiles[0].is4K() {
		t.Error("an allowed 1080p profile with 2160p disallowed is not a 4K profile")
	}
	if !profiles[1].is4K() {
		t.Error("a profile that allows 2160p is a 4K profile")
	}
}

func TestListArrQualityProfilesReadsTheServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/qualityprofile" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Api-Key"); got != "secret" {
			t.Errorf("api key = %q", got)
		}
		_, _ = w.Write([]byte(trashProfileJSON))
	}))
	defer srv.Close()

	profiles, err := listArrQualityProfiles(context.Background(), srv.URL, "secret")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(profiles) != 2 || profiles[0].Name != "French HD 1080p" {
		t.Fatalf("profiles = %+v", profiles)
	}
}

func TestListArrQualityProfilesReportsHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	if _, err := listArrQualityProfiles(context.Background(), srv.URL, "bad"); err == nil {
		t.Fatal("want an error for a refused key")
	}
}

func TestProfileAllowedByAllowlist(t *testing.T) {
	if !profileAllowedByAllowlist(nil, "c-1", 4) {
		t.Error("an empty allowlist allows every profile")
	}
	if !profileAllowedByAllowlist([]string{"c-1:4", "c-2:9"}, "c-1", 4) {
		t.Error("a listed profile is allowed")
	}
	if profileAllowedByAllowlist([]string{"c-2:4"}, "c-1", 4) {
		t.Error("the same profile id on another server is not allowed")
	}
}

func TestApplyRequesterProfileGivesTheHDCopyA1080Profile(t *testing.T) {
	profileID := 4
	req := Request{QualityProfileID: &profileID, QualityProfileConnectionID: "c-1"}
	conns := []ResolvedRouterConnection{
		{ID: "c-1", Config: map[string]any{configQualityProfileID: float64(2)}},
	}
	got := applyRequesterProfile(req, conns, Quality1080p)
	if len(got) != 1 {
		t.Fatalf("conns = %d", len(got))
	}
	if got[0].Config[configQualityProfileID] != float64(4) {
		t.Errorf("HD profile = %v, want 4", got[0].Config[configQualityProfileID])
	}
	// The original connection is untouched.
	if conns[0].Config[configQualityProfileID] != float64(2) {
		t.Error("applyRequesterProfile mutated its input")
	}
}

func TestApplyRequesterProfileLeavesThe4KCopyAlone(t *testing.T) {
	profileID := 4
	req := Request{QualityProfileID: &profileID, QualityProfileConnectionID: "c-1"}
	conn := ResolvedRouterConnection{ID: "c-1", Config: map[string]any{configQualityProfileID: float64(9)}}
	got := applyRequesterProfile(req, []ResolvedRouterConnection{conn}, Quality2160p)
	if got[0].Config[configQualityProfileID] != float64(9) {
		t.Errorf("4K profile = %v, want the server's 9", got[0].Config[configQualityProfileID])
	}
}

func TestApplyRequesterProfileGivesThe4KCopyA4KProfile(t *testing.T) {
	profileID := 4
	req := Request{QualityProfileID: &profileID, QualityProfile4K: true, QualityProfileConnectionID: "c-1"}
	hd := ResolvedRouterConnection{ID: "c-1", Config: map[string]any{configQualityProfileID: float64(2)}}
	uhd := ResolvedRouterConnection{ID: "c-1", Config: map[string]any{configQualityProfileID: float64(9)}}

	if got := applyRequesterProfile(req, []ResolvedRouterConnection{hd}, Quality1080p); got[0].Config[configQualityProfileID] != float64(2) {
		t.Errorf("HD profile = %v, want the server's 2, so a 4K pick still downloads the 1080 copy", got[0].Config[configQualityProfileID])
	}
	if got := applyRequesterProfile(req, []ResolvedRouterConnection{uhd}, Quality2160p); got[0].Config[configQualityProfileID] != float64(4) {
		t.Errorf("4K profile = %v, want the picked 4", got[0].Config[configQualityProfileID])
	}
}

func TestApplyRequesterProfileFallsBackToTheTierDefault(t *testing.T) {
	profileID := 4
	req := Request{QualityProfileID: &profileID, QualityProfileConnectionID: "c-gone"}
	conns := []ResolvedRouterConnection{
		{ID: "hd", Config: map[string]any{configIsDefault: true, configQualityProfileID: float64(2)}},
		{ID: "uhd", Config: map[string]any{configIsDefault: true, configIsDefault4K: true, configQualityProfileID: float64(9)}},
	}
	got := applyRequesterProfile(req, conns, Quality1080p)
	if got[0].Config[configQualityProfileID] != float64(4) {
		t.Errorf("hd default profile = %v, want 4", got[0].Config[configQualityProfileID])
	}
	if got[1].Config[configQualityProfileID] != float64(9) {
		t.Errorf("uhd profile = %v, want its own 9", got[1].Config[configQualityProfileID])
	}
}

func TestApplyRequesterProfileDoesNothingWithoutAPick(t *testing.T) {
	conns := []ResolvedRouterConnection{{ID: "c-1", Config: map[string]any{configQualityProfileID: float64(2)}}}
	if got := applyRequesterProfile(Request{}, conns, Quality1080p); got[0].Config[configQualityProfileID] != float64(2) {
		t.Errorf("a request without a pick keeps the server profile, got %v", got[0].Config[configQualityProfileID])
	}
}

func TestAllowedQualitiesFollowThePickedProfile(t *testing.T) {
	// Force-dual would give a 4K copy to everyone; a request that picked a
	// 1080 profile must not get one anyway.
	store := &fakeStore{settings: Settings{RequestsEnabled: true, ForceDualQuality: true}}
	service := newTestService(store)
	profileID := 4

	hd, _ := service.allowedQualities(context.Background(), Request{RequestedByUserID: 1, QualityProfileID: &profileID}, store.settings)
	if len(hd) != 1 || hd[0] != Quality1080p {
		t.Fatalf("qualities = %v, want HD only", hd)
	}

	dual, _ := service.allowedQualities(context.Background(), Request{RequestedByUserID: 1, QualityProfileID: &profileID, QualityProfile4K: true}, store.settings)
	if len(dual) != 2 || dual[0] != Quality1080p || dual[1] != Quality2160p {
		t.Fatalf("qualities = %v, want HD and 4K", dual)
	}

	// Untouched: a request with no pick keeps the server's policy.
	legacy, _ := service.allowedQualities(context.Background(), Request{RequestedByUserID: 1}, store.settings)
	if len(legacy) != 2 {
		t.Fatalf("qualities = %v, want force-dual to still ask for both", legacy)
	}
}

func TestRequesterProfileChoicesListsProfilesAndHides4K(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(trashProfileJSON))
	}))
	defer srv.Close()

	store := &fakeStore{settings: Settings{RequestsEnabled: true, RequesterQualityProfiles: true}}
	store.integrations = []Integration{{
		ID: "c-1", Name: "Radarr", Enabled: true, BaseURL: srv.URL, APIKeyRef: "key",
		CapabilityID: "request_router.v1/sonarr-radarr", InstallationID: intPtr(1),
		SupportedMediaTypes: []string{"movie"},
		PluginConfig:        map[string]any{configQualityProfileID: float64(3)},
	}}
	service := newTestService(store)

	choices, err := service.RequesterProfileChoices(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, MediaTypeMovie)
	if err != nil {
		t.Fatalf("choices: %v", err)
	}
	if !choices.Enabled || len(choices.Servers) != 1 {
		t.Fatalf("choices = %+v", choices)
	}
	profiles := choices.Servers[0].Profiles
	if len(profiles) != 2 {
		t.Fatalf("profiles = %+v", profiles)
	}
	if profiles[0].ID != 3 || profiles[0].Is4K || !profiles[0].Default {
		t.Errorf("first profile = %+v, want the 1080 default", profiles[0])
	}
	if profiles[1].ID != 4 || !profiles[1].Is4K || profiles[1].Default {
		t.Errorf("second profile = %+v, want the 4K one", profiles[1])
	}

	// An admin allowlist narrows the offer to what they curate.
	store.settings.RequesterQualityProfileAllowlist = []string{"c-1:4"}
	narrowed, err := service.RequesterProfileChoices(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, MediaTypeMovie)
	if err != nil {
		t.Fatalf("narrowed: %v", err)
	}
	if got := narrowed.Servers[0].Profiles; len(got) != 1 || got[0].ID != 4 {
		t.Fatalf("profiles = %+v, want only the allowed one", got)
	}

	// Turned off, the dialog is offered nothing.
	store.settings.RequesterQualityProfiles = false
	off, err := service.RequesterProfileChoices(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, MediaTypeMovie)
	if err != nil {
		t.Fatalf("off: %v", err)
	}
	if off.Enabled || len(off.Servers) != 0 {
		t.Fatalf("choices = %+v, want nothing offered", off)
	}
}
