package iamlogin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Field names on the wire, as iam-login's OidcClient record reads them.
func TestClientFieldsMatchTheAPI(t *testing.T) {
	body, _ := json.Marshal(OIDCClient{ID: "incus", PublicClient: true, DeviceAuthorization: true, Audiences: []string{"https://incus"}})
	var wire map[string]any
	_ = json.Unmarshal(body, &wire)
	for _, field := range []string{"publicClient", "deviceAuthorization", "audiences", "pkceRequired", "loginEnabled"} {
		if _, ok := wire[field]; !ok {
			t.Errorf("missing %s in %s", field, body)
		}
	}
	// Omitted unless set, so the server applies its default (RFC 7591: client_secret_basic).
	if _, ok := wire["tokenEndpointAuthMethod"]; ok {
		t.Errorf("unset tokenEndpointAuthMethod sent: %s", body)
	}
	post, _ := json.Marshal(OIDCClient{ID: "ak", TokenEndpointAuthMethod: "client_secret_post"})
	if !strings.Contains(string(post), `"tokenEndpointAuthMethod":"client_secret_post"`) {
		t.Errorf("tokenEndpointAuthMethod not sent: %s", post)
	}
	var created struct {
		Client OIDCClient `json:"client"`
		Secret string     `json:"secret"`
	}
	if err := json.Unmarshal([]byte(`{"client":{"id":"incus","publicClient":true,"audiences":["https://incus"]},"secret":null}`), &created); err != nil ||
		!created.Client.PublicClient || created.Secret != "" || created.Client.Audiences[0] != "https://incus" {
		t.Fatalf("public client response: %+v %v", created, err)
	}
}

// The wire contract with iam-login's /api/iam/clients (docs/client-management.md).
func TestClientSpeaksTheManagementAPI(t *testing.T) {
	stored := map[string]OIDCClient{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer svid" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/iam/clients":
			var c OIDCClient
			_ = json.NewDecoder(r.Body).Decode(&c)
			stored[c.ID] = c
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"client": c, "secret": "first"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/iam/clients/app":
			c, ok := stored["app"]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(c)
		case r.Method == http.MethodPost && r.URL.Path == "/api/iam/clients/app/secret":
			_ = json.NewEncoder(w).Encode(map[string]string{"secret": "second"})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/iam/clients/app":
			delete(stored, "app")
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer server.Close()
	api, err := New(server.URL+"/", func(context.Context) (string, error) { return "svid", nil }, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	want := OIDCClient{ID: "app", Name: "App", RedirectURIs: []string{"https://app/cb"}, Scopes: []string{"openid"},
		PostLogoutRedirectURIs: []string{}, LoginEnabled: true, PKCERequired: false}
	created, secret, err := api.Create(ctx, want)
	if err != nil || secret != "first" || created.PKCERequired || created.ID != "app" {
		t.Fatalf("create: %+v %q %v", created, secret, err)
	}
	if got, err := api.Get(ctx, "app"); err != nil || got.Name != "App" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if secret, err := api.RotateSecret(ctx, "app"); err != nil || secret != "second" {
		t.Fatalf("rotate: %q %v", secret, err)
	}
	if err := api.Delete(ctx, "app"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := api.Get(ctx, "app"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
	failing, _ := New(server.URL, func(context.Context) (string, error) { return "", errors.New("no workload api") }, server.Client())
	if _, err := failing.Get(ctx, "app"); err == nil {
		t.Fatal("a missing SPIFFE identity must fail the request")
	}
}

// The wire contract with iam-login's /api/iam/machine-clients (MachineClient in docs/openapi.yaml).
func TestClientSpeaksTheMachineClientAPI(t *testing.T) {
	stored := map[string]MachineClient{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/iam/machine-clients":
			var wire map[string]any
			_ = json.NewDecoder(r.Body).Decode(&wire)
			if wire["spiffeId"] != "spiffe://ddupan.top/ci/*" || wire["enabled"] != true {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			c := MachineClient{ID: wire["id"].(string), Name: wire["name"].(string), SPIFFEID: wire["spiffeId"].(string), Enabled: true}
			stored[c.ID] = c
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(c)
		case r.Method == http.MethodPut && r.URL.Path == "/api/iam/machine-clients/ci":
			var c MachineClient
			_ = json.NewDecoder(r.Body).Decode(&c)
			stored["ci"] = c
			_ = json.NewEncoder(w).Encode(c)
		case r.Method == http.MethodGet && r.URL.Path == "/api/iam/machine-clients/ci":
			c, ok := stored["ci"]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(c)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/iam/machine-clients/ci":
			delete(stored, "ci")
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer server.Close()
	api, _ := New(server.URL, func(context.Context) (string, error) { return "svid", nil }, server.Client())
	ctx := context.Background()
	want := MachineClient{ID: "ci", Name: "CI", SPIFFEID: "spiffe://ddupan.top/ci/*", Scopes: []string{}, Audiences: []string{"artifact-keeper"}, Enabled: true}
	if created, err := api.CreateMachine(ctx, want); err != nil || created.SPIFFEID != want.SPIFFEID {
		t.Fatalf("create: %+v %v", created, err)
	}
	want.Enabled = false
	if updated, err := api.UpdateMachine(ctx, want); err != nil || updated.Enabled {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if got, err := api.GetMachine(ctx, "ci"); err != nil || got.Enabled || got.Audiences[0] != "artifact-keeper" {
		t.Fatalf("get: %+v %v", got, err)
	}
	if err := api.DeleteMachine(ctx, "ci"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := api.GetMachine(ctx, "ci"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
}
