package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"git.ddupan.top/panxiao81/terraform-provider-homelab/internal/iamlogin"
)

func (r *oidcClientResource) toAPI(ctx context.Context, m oidcClientModel, diags *diag.Diagnostics) iamlogin.OIDCClient {
	client := iamlogin.OIDCClient{
		ID: m.ID.ValueString(), Name: m.Name.ValueString(),
		LoginEnabled: m.LoginEnabled.ValueBool(), PKCERequired: m.PKCERequired.ValueBool(),
		RedirectURIs: []string{}, Scopes: []string{}, PostLogoutRedirectURIs: []string{},
	}
	diags.Append(m.RedirectURIs.ElementsAs(ctx, &client.RedirectURIs, false)...)
	diags.Append(m.Scopes.ElementsAs(ctx, &client.Scopes, false)...)
	diags.Append(m.PostLogoutRedirectURIs.ElementsAs(ctx, &client.PostLogoutRedirectURIs, false)...)
	return client
}

// fromAPI overwrites the managed fields with the server's view; secret and secret_rotation are left alone.
func (r *oidcClientResource) fromAPI(ctx context.Context, c iamlogin.OIDCClient, m *oidcClientModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(c.ID)
	m.Name = types.StringValue(c.Name)
	m.LoginEnabled = types.BoolValue(c.LoginEnabled)
	m.PKCERequired = types.BoolValue(c.PKCERequired)
	var d diag.Diagnostics
	m.RedirectURIs, d = types.ListValueFrom(ctx, types.StringType, nonNil(c.RedirectURIs))
	diags.Append(d...)
	m.Scopes, d = types.SetValueFrom(ctx, types.StringType, nonNil(c.Scopes))
	diags.Append(d...)
	m.PostLogoutRedirectURIs, d = types.ListValueFrom(ctx, types.StringType, nonNil(c.PostLogoutRedirectURIs))
	diags.Append(d...)
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
