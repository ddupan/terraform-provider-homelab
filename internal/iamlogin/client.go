// Package iamlogin is the HTTP client for iam-login's client-management API (/api/iam/clients).
package iamlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ErrNotFound is returned when the client does not exist (404).
var ErrNotFound = errors.New("client not found")

// TokenSource returns a bearer token for one request. Tokens are short-lived JWT-SVIDs, so a fresh one
// per request avoids expiry in long applies.
type TokenSource func(ctx context.Context) (string, error)

// OIDCClient mirrors the API's client representation.
type OIDCClient struct {
	ID                     string   `json:"id"`
	Name                   string   `json:"name"`
	RedirectURIs           []string `json:"redirectUris"`
	Scopes                 []string `json:"scopes"`
	PostLogoutRedirectURIs []string `json:"postLogoutRedirectUris"`
	LoginEnabled           bool     `json:"loginEnabled"`
	PKCERequired           bool     `json:"pkceRequired"`
	PublicClient           bool     `json:"publicClient"`
	DeviceAuthorization    bool     `json:"deviceAuthorization"`
	Audiences              []string `json:"audiences"`
	// RFC 7591 token_endpoint_auth_method; omitted means the server's default (client_secret_basic, none if public).
	TokenEndpointAuthMethod string `json:"tokenEndpointAuthMethod,omitempty"`
}

type Client struct {
	endpoint *url.URL
	http     *http.Client
	token    TokenSource
}

func New(endpoint string, token TokenSource, httpClient *http.Client) (*Client, error) {
	u, err := url.Parse(strings.TrimSuffix(endpoint, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid iam-login endpoint %q", endpoint)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{endpoint: u, http: httpClient, token: token}, nil
}

// Create registers a client; the secret is returned only here (and by RotateSecret), and is empty for a public client.
func (c *Client) Create(ctx context.Context, client OIDCClient) (OIDCClient, string, error) {
	var created struct {
		Client OIDCClient `json:"client"`
		Secret string     `json:"secret"`
	}
	err := c.do(ctx, http.MethodPost, "/api/iam/clients", client, http.StatusCreated, &created)
	return created.Client, created.Secret, err
}

func (c *Client) Get(ctx context.Context, id string) (OIDCClient, error) {
	var client OIDCClient
	err := c.do(ctx, http.MethodGet, "/api/iam/clients/"+url.PathEscape(id), nil, http.StatusOK, &client)
	return client, err
}

func (c *Client) Update(ctx context.Context, client OIDCClient) (OIDCClient, error) {
	var updated OIDCClient
	err := c.do(ctx, http.MethodPut, "/api/iam/clients/"+url.PathEscape(client.ID), client, http.StatusOK, &updated)
	return updated, err
}

func (c *Client) Delete(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/iam/clients/"+url.PathEscape(id), nil, http.StatusNoContent, nil)
}

// RotateSecret replaces the secret; the old one stops working immediately.
func (c *Client) RotateSecret(ctx context.Context, id string) (string, error) {
	var rotated struct {
		Secret string `json:"secret"`
	}
	err := c.do(ctx, http.MethodPost, "/api/iam/clients/"+url.PathEscape(id)+"/secret", nil, http.StatusOK, &rotated)
	return rotated.Secret, err
}

func (c *Client) do(ctx context.Context, method, path string, body any, want int, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.endpoint.String()+path, reader)
	if err != nil {
		return err
	}
	token, err := c.token(ctx)
	if err != nil {
		return fmt.Errorf("obtaining a token for iam-login: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if response.StatusCode != want {
		// Error bodies never contain secrets; keep a bounded excerpt for diagnosis.
		excerpt, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(excerpt)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}
