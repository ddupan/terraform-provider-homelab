package iamlogin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
