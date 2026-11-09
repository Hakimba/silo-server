package requests

import (
	"context"
	"errors"
	"testing"
)

// seerrProfiles is what the Seerr plugin publishes for one connection: the
// server's own profile first, then the profiles it read from Radarr.
func seerrProfiles() map[string][]RouterOption {
	return map[string][]RouterOption{
		profileOptionFieldRadarr: {
			{Value: "default", Label: "Seerr default (French HD 1080p)"},
			{Value: "4", Label: "French HD 1080p"},
			{Value: "5", Label: "French HD 720p Light"},
			{Value: "6", Label: "French 4K Remux"},
		},
	}
}

// profileStore is a store whose one connection is a Seerr request router.
func profileStore(settings Settings) *fakeStore {
	store := &fakeStore{settings: settings}
	store.integrations = []Integration{{
		ID: "c-seerr", Name: "Seerr", Enabled: true, BaseURL: "http://seerr", APIKeyRef: "key",
		CapabilityID: "request_router.v1/seerr", InstallationID: intPtr(1),
		SupportedMediaTypes: []string{"movie", "series"},
		PluginConfig:        map[string]any{},
	}}
	return store
}

func TestRequesterProfileChoicesAsksThePlugin(t *testing.T) {
	store := profileStore(Settings{RequestsEnabled: true, RequesterQualityProfiles: true})
	service, router := newRoutedService(store)
	router.options = seerrProfiles()

	choices, err := service.RequesterProfileChoices(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, MediaTypeMovie)
	if err != nil {
		t.Fatalf("choices: %v", err)
	}
	if !choices.Enabled || len(choices.Servers) != 1 {
		t.Fatalf("choices = %+v", choices)
	}
	server := choices.Servers[0]
	if server.ConnectionID != "c-seerr" || server.ServerName != "Seerr" {
		t.Fatalf("server = %+v", server)
	}
	if len(server.Profiles) != 4 {
		t.Fatalf("profiles = %+v", server.Profiles)
	}
	if server.Profiles[0].ID != 0 || !server.Profiles[0].Default ||
		server.Profiles[0].Name != "Seerr default (French HD 1080p)" {
		t.Errorf("first choice = %+v, want the server default", server.Profiles[0])
	}
	if server.Profiles[2].ID != 5 || server.Profiles[2].Name != "French HD 720p Light" {
		t.Errorf("third choice = %+v", server.Profiles[2])
	}
	// The host hands the plugin the connection it stores, so the plugin can ask
	// its own backend.
	if router.gotOptionsConn.ID != "c-seerr" || router.gotOptionsConn.BaseURL != "http://seerr" {
		t.Errorf("options connection = %+v", router.gotOptionsConn)
	}
}

func TestRequesterProfileChoicesHidesWhenTheAdminSwitchIsOff(t *testing.T) {
	store := profileStore(Settings{RequestsEnabled: true})
	service, router := newRoutedService(store)
	router.options = seerrProfiles()

	choices, err := service.RequesterProfileChoices(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, MediaTypeMovie)
	if err != nil {
		t.Fatalf("choices: %v", err)
	}
	if choices.Enabled || len(choices.Servers) != 0 {
		t.Fatalf("choices = %+v, want nothing offered", choices)
	}
	if router.gotOptionsConn.ID != "" {
		t.Error("the plugin was asked for profiles while the feature is off")
	}
}

func TestRequesterProfileChoicesFollowsTheAllowlist(t *testing.T) {
	store := profileStore(Settings{
		RequestsEnabled:                  true,
		RequesterQualityProfiles:         true,
		RequesterQualityProfileAllowlist: []string{"c-seerr:5"},
	})
	service, router := newRoutedService(store)
	router.options = seerrProfiles()

	choices, err := service.RequesterProfileChoices(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, MediaTypeMovie)
	if err != nil {
		t.Fatalf("choices: %v", err)
	}
	profiles := choices.Servers[0].Profiles
	// The default stays: it is "no choice", not a profile.
	if len(profiles) != 2 || profiles[0].ID != 0 || profiles[1].ID != 5 {
		t.Fatalf("profiles = %+v, want the default and profile 5", profiles)
	}
}

func TestRequesterProfileChoicesSkipsAPluginThatFails(t *testing.T) {
	store := profileStore(Settings{RequestsEnabled: true, RequesterQualityProfiles: true})
	service, router := newRoutedService(store)
	router.optionsErr = errors.New("seerr unreachable")

	choices, err := service.RequesterProfileChoices(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, MediaTypeMovie)
	if err != nil {
		t.Fatalf("an unreachable server must not fail the read: %v", err)
	}
	if choices.Enabled || len(choices.Servers) != 0 {
		t.Fatalf("choices = %+v, want the server left out", choices)
	}
}

func TestResolveRequesterQualityProfileCarriesTheNameAndConnection(t *testing.T) {
	store := profileStore(Settings{RequestsEnabled: true, RequesterQualityProfiles: true})
	service, router := newRoutedService(store)
	router.options = seerrProfiles()

	input := CreateRequestInput{MediaType: MediaTypeMovie, QualityProfileID: intPtr(5)}
	if err := service.resolveRequesterQualityProfile(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, &input); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if input.QualityProfileName != "French HD 720p Light" || input.QualityProfileConnectionID != "c-seerr" {
		t.Fatalf("input = %+v", input)
	}
}

func TestResolveRequesterQualityProfileRefusesAnUnknownProfile(t *testing.T) {
	store := profileStore(Settings{RequestsEnabled: true, RequesterQualityProfiles: true})
	service, router := newRoutedService(store)
	router.options = seerrProfiles()

	input := CreateRequestInput{MediaType: MediaTypeMovie, QualityProfileID: intPtr(99)}
	err := service.resolveRequesterQualityProfile(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, &input)
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("err = %v, want a field error", err)
	}
	if validation.FieldErrors[fieldQualityProfileID] == "" {
		t.Errorf("field errors = %+v, want the profile field", validation.FieldErrors)
	}
}

func TestResolveRequesterQualityProfileTurnsTheDefaultIntoNoChoice(t *testing.T) {
	store := profileStore(Settings{RequestsEnabled: true, RequesterQualityProfiles: true})
	service, router := newRoutedService(store)
	router.options = seerrProfiles()

	input := CreateRequestInput{MediaType: MediaTypeMovie, QualityProfileID: intPtr(0)}
	if err := service.resolveRequesterQualityProfile(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, &input); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if input.QualityProfileID != nil {
		t.Fatalf("quality profile = %v, want none so the server keeps its own", *input.QualityProfileID)
	}
}

func TestResolveRequesterQualityProfileRefusesWhenTurnedOff(t *testing.T) {
	store := profileStore(Settings{RequestsEnabled: true})
	service, router := newRoutedService(store)
	router.options = seerrProfiles()

	input := CreateRequestInput{MediaType: MediaTypeMovie, QualityProfileID: intPtr(4)}
	var validation *ValidationError
	if err := service.resolveRequesterQualityProfile(context.Background(), Viewer{UserID: 1, ProfileID: "p-1"}, &input); !errors.As(err, &validation) {
		t.Fatalf("err = %v, want a field error", err)
	}
}

func TestApplyRequesterProfileWritesThePickedProfile(t *testing.T) {
	conns := []ResolvedRouterConnection{
		{ID: "c-seerr", Config: map[string]any{"radarr_server_id": "0"}},
		{ID: "c-other", Config: map[string]any{"radarr_server_id": "1"}},
	}
	req := Request{QualityProfileID: intPtr(5), QualityProfileConnectionID: "c-seerr"}
	got := applyRequesterProfile(req, conns)
	if got[0].Config[profileOptionFieldGeneric] != float64(5) {
		t.Errorf("picked connection = %v, want the profile", got[0].Config[profileOptionFieldGeneric])
	}
	if _, ok := got[1].Config[profileOptionFieldGeneric]; ok {
		t.Error("a sibling server got the profile too")
	}
	// The stored connection config must not be mutated in place.
	if _, ok := conns[0].Config[profileOptionFieldGeneric]; ok {
		t.Error("applyRequesterProfile mutated its input")
	}
	// The server id survives: only the profile key is added.
	if got[0].Config["radarr_server_id"] != "0" {
		t.Errorf("server id = %v, want it kept", got[0].Config["radarr_server_id"])
	}
}

func TestApplyRequesterProfileDoesNothingWithoutAPick(t *testing.T) {
	conns := []ResolvedRouterConnection{{ID: "c-seerr", Config: map[string]any{}}}
	got := applyRequesterProfile(Request{}, conns)
	if _, ok := got[0].Config[profileOptionFieldGeneric]; ok {
		t.Error("a request without a pick must leave the server's profile alone")
	}
}
