package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ddupan/terraform-provider-homelab/internal/iamlogin"
)

func (r *oidcClientResource) toAPI(ctx context.Context, m oidcClientModel, diags *diag.Diagnostics) iamlogin.OIDCClient {
	client := iamlogin.OIDCClient{
		ID: m.ID.ValueString(), Name: m.Name.ValueString(),
		LoginEnabled: m.LoginEnabled.ValueBool(), PKCERequired: m.PKCERequired.ValueBool(),
		PublicClient: m.PublicClient.ValueBool(), DeviceAuthorization: m.DeviceAuthorization.ValueBool(),
		RedirectURIs: []string{}, Scopes: []string{}, PostLogoutRedirectURIs: []string{}, Audiences: []string{},
	}
	diags.Append(m.RedirectURIs.ElementsAs(ctx, &client.RedirectURIs, false)...)
	diags.Append(m.Scopes.ElementsAs(ctx, &client.Scopes, false)...)
	diags.Append(m.PostLogoutRedirectURIs.ElementsAs(ctx, &client.PostLogoutRedirectURIs, false)...)
	diags.Append(m.Audiences.ElementsAs(ctx, &client.Audiences, false)...)
	if !m.TokenEndpointAuthMethod.IsNull() && !m.TokenEndpointAuthMethod.IsUnknown() {
		client.TokenEndpointAuthMethod = m.TokenEndpointAuthMethod.ValueString()
	}
	return client
}

// fromAPI overwrites the managed fields with the server's view; secret and secret_rotation are left alone.
func (r *oidcClientResource) fromAPI(ctx context.Context, c iamlogin.OIDCClient, m *oidcClientModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(c.ID)
	m.Name = types.StringValue(c.Name)
	m.LoginEnabled = types.BoolValue(c.LoginEnabled)
	m.PKCERequired = types.BoolValue(c.PKCERequired)
	m.PublicClient = types.BoolValue(c.PublicClient)
	m.DeviceAuthorization = types.BoolValue(c.DeviceAuthorization)
	m.TokenEndpointAuthMethod = types.StringValue(c.TokenEndpointAuthMethod)
	var d diag.Diagnostics
	m.RedirectURIs, d = types.SetValueFrom(ctx, types.StringType, nonNil(c.RedirectURIs))
	diags.Append(d...)
	m.Scopes, d = types.SetValueFrom(ctx, types.StringType, nonNil(c.Scopes))
	diags.Append(d...)
	m.PostLogoutRedirectURIs, d = types.SetValueFrom(ctx, types.StringType, nonNil(c.PostLogoutRedirectURIs))
	diags.Append(d...)
	m.Audiences, d = types.SetValueFrom(ctx, types.StringType, nonNil(c.Audiences))
	diags.Append(d...)
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
