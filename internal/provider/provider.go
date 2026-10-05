// Package provider is the homelab Terraform provider: one provider for the platform's own APIs, each
// backend configured in its own block and authenticated with the caller's SPIFFE identity.
package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/spiffe/go-spiffe/v2/svid/jwtsvid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"

	"git.ddupan.top/panxiao81/terraform-provider-homelab/internal/iamlogin"
)

// iamLoginAudience is the audience iam-login requires of JWT-SVIDs.
const iamLoginAudience = "iam-login"

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &homelab{version: version} }
}

type homelab struct{ version string }

type providerModel struct {
	SpiffeEndpointSocket types.String   `tfsdk:"spiffe_endpoint_socket"`
	IAMLogin             *iamLoginModel `tfsdk:"iam_login"`
}

type iamLoginModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
}

func (p *homelab) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "homelab"
	resp.Version = p.version
}

func (p *homelab) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Homelab platform APIs. Every backend is called with a JWT-SVID from the local SPIFFE Workload API; " +
			"the provider holds no credentials.",
		Attributes: map[string]schema.Attribute{
			"spiffe_endpoint_socket": schema.StringAttribute{
				Optional:    true,
				Description: "SPIFFE Workload API address, e.g. unix:///run/spire-agent/agent.sock. Defaults to SPIFFE_ENDPOINT_SOCKET.",
			},
			"iam_login": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "iam-login (OIDC clients). The caller's SPIFFE ID must be in iam-login's iam.clients.spiffe.admin-ids.",
				Attributes: map[string]schema.Attribute{
					"endpoint": schema.StringAttribute{Required: true, Description: "Base URL, e.g. https://sso.ddupan.top."},
				},
			},
		},
	}
}

// clients is what resources receive from Configure; a nil backend means its block was not configured.
type clients struct{ iamLogin *iamlogin.Client }

func (p *homelab) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	socket := config.SpiffeEndpointSocket.ValueString()
	if socket == "" {
		socket = os.Getenv("SPIFFE_ENDPOINT_SOCKET")
	}
	var configured clients
	if config.IAMLogin != nil {
		client, err := iamlogin.New(config.IAMLogin.Endpoint.ValueString(), svidSource(socket, iamLoginAudience), nil)
		if err != nil {
			resp.Diagnostics.AddError("Invalid iam_login configuration", err.Error())
			return
		}
		configured.iamLogin = client
	}
	resp.ResourceData = configured
	resp.DataSourceData = configured
}

// svidSource fetches a fresh JWT-SVID per call; SVIDs are short-lived and applies can be long.
func svidSource(socket, audience string) iamlogin.TokenSource {
	return func(ctx context.Context) (string, error) {
		var options []workloadapi.ClientOption
		if socket != "" {
			options = append(options, workloadapi.WithAddr(socket))
		}
		svid, err := workloadapi.FetchJWTSVID(ctx, jwtsvid.Params{Audience: audience}, options...)
		if err != nil {
			return "", err
		}
		return svid.Marshal(), nil
	}
}

func (p *homelab) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{newOIDCClientResource}
}

func (p *homelab) DataSources(context.Context) []func() datasource.DataSource { return nil }
