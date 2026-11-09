package apiv2

import (
	"context"
	"net/http"

	mediarequests "github.com/Silo-Server/silo-server/internal/requests"
)

// opRequestProfileOptions is the v2 operation id of the quality profile
// picker's read.
const opRequestProfileOptions = "getRequestProfileOptions"

// RequestProfileOptionsInput is the getRequestProfileOptions query.
type RequestProfileOptionsInput struct {
	MediaType string `query:"media_type" required:"true" enum:"movie,series" doc:"Which request's profiles to list: movies come from Radarr, series from Sonarr" example:"movie"`
}

// RequestProfileOption is one quality profile a requester may pick.
type RequestProfileOption struct {
	ID      int    `json:"id" example:"4"`
	Name    string `json:"name" example:"French HD 1080p"`
	Is4K    bool   `json:"is_4k" doc:"The profile allows 2160p, so picking it asks for a 4K copy as well as the 1080 one" example:"false"`
	Default bool   `json:"default" doc:"The profile the server downloads with when the requester picks nothing" example:"true"`
}

// RequestProfileServer is one download server's offer.
type RequestProfileServer struct {
	ConnectionID string                 `json:"connection_id" doc:"Send as quality_profile_connection_id when two servers share a profile id" example:"c-9f2"`
	ServerName   string                 `json:"server_name" example:"Radarr"`
	Profiles     []RequestProfileOption `json:"profiles"`
}

// RequestProfileOptionsOutput lists what a request may download with.
type RequestProfileOptionsOutput struct {
	Body RequestProfileOptions
}

// RequestProfileOptions is the profile picker's data. Enabled is false when
// the admin has not let requesters choose, or no server answered.
type RequestProfileOptions struct {
	Enabled bool                   `json:"enabled" example:"true"`
	Servers []RequestProfileServer `json:"servers"`
}

// getRequestProfileOptions lists the quality profiles the viewer's requests may
// download with: the Radarr ones for a movie, the Sonarr ones for a series.
func (reg *Registry) getRequestProfileOptions(ctx context.Context, in *RequestProfileOptionsInput) (*RequestProfileOptionsOutput, error) {
	svc, viewer, p := reg.requestViewer(ctx)
	if p != nil {
		return nil, p
	}
	choices, err := svc.RequesterProfileChoices(ctx, viewer, mediarequests.MediaType(in.MediaType))
	if err != nil {
		return nil, requestProblem(err)
	}
	out := RequestProfileOptions{Enabled: choices.Enabled, Servers: make([]RequestProfileServer, 0, len(choices.Servers))}
	for _, server := range choices.Servers {
		profiles := make([]RequestProfileOption, 0, len(server.Profiles))
		for _, profile := range server.Profiles {
			profiles = append(profiles, RequestProfileOption{
				ID: profile.ID, Name: profile.Name, Is4K: profile.Is4K, Default: profile.Default,
			})
		}
		out.Servers = append(out.Servers, RequestProfileServer{
			ConnectionID: server.ConnectionID, ServerName: server.ServerName, Profiles: profiles,
		})
	}
	return &RequestProfileOptionsOutput{Body: out}, nil
}

// registerRequestProfileOptions registers the picker's read.
func registerRequestProfileOptions(reg *Registry) {
	op := humaOp(http.MethodGet, Prefix+"/requests/profile-options", opRequestProfileOptions, requestsTag,
		"List the quality profiles a request may download with, and whether requesters may choose one.")
	op.Errors = []int{http.StatusConflict}
	Register(reg, Operation{Operation: op, Class: ClassProfileScoped, ServiceBacked: true}, reg.getRequestProfileOptions)
}
