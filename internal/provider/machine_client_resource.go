package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ddupan/terraform-provider-homelab/internal/iamlogin"
)

func newMachineClientResource() resource.Resource { return &machineClientResource{} }

type machineClientResource struct{ api *iamlogin.Client }

type machineClientModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	SPIFFEID  types.String `tfsdk:"spiffe_id"`
	Scopes    types.Set    `tfsdk:"scopes"`
	Audiences types.Set    `tfsdk:"audiences"`
	Enabled   types.Bool   `tfsdk:"enabled"`
}

func (r *machineClientResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_client"
}

func (r *machineClientResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A machine client of iam-login: authenticates with its SPIFFE JWT-SVID (spiffe_jwt) and gets " +
			"client_credentials tokens. No secret.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true, Description: "Client ID, sent as client_id at the token endpoint. Changing it replaces the client.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Required: true},
			"spiffe_id": schema.StringAttribute{
				Required:    true,
				Description: "SPIFFE ID of the workload; ending in /* admits every ID below that path (whole segments only).",
			},
			"scopes": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
				Description: "Scopes the client may request.",
			},
			"audiences": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, nil)),
				Description: "Extra access-token audiences, for resources that check aud.",
			},
			"enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "false stops issuing tokens to the client.",
			},
		},
	}
}

func (r *machineClientResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	configured, ok := req.ProviderData.(clients)
	if !ok || configured.iamLogin == nil {
		resp.Diagnostics.AddError("iam_login is not configured", "homelab_machine_client needs the provider's iam_login block.")
		return
	}
	r.api = configured.iamLogin
}

func (r *machineClientResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan machineClientModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	client := machineToAPI(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.api.CreateMachine(ctx, client)
	if err != nil {
		resp.Diagnostics.AddError("Creating machine client "+client.ID, err.Error())
		return
	}
	machineFromAPI(ctx, created, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *machineClientResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state machineClientModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.api.GetMachine(ctx, state.ID.ValueString())
	if errors.Is(err, iamlogin.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading machine client "+state.ID.ValueString(), err.Error())
		return
	}
	machineFromAPI(ctx, current, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *machineClientResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan machineClientModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	client := machineToAPI(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.api.UpdateMachine(ctx, client)
	if err != nil {
		resp.Diagnostics.AddError("Updating machine client "+client.ID, err.Error())
		return
	}
	machineFromAPI(ctx, updated, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *machineClientResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state machineClientModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.DeleteMachine(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, iamlogin.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting machine client "+state.ID.ValueString(), err.Error())
	}
}

func (r *machineClientResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func machineToAPI(ctx context.Context, m machineClientModel, diags *diag.Diagnostics) iamlogin.MachineClient {
	client := iamlogin.MachineClient{
		ID: m.ID.ValueString(), Name: m.Name.ValueString(), SPIFFEID: m.SPIFFEID.ValueString(),
		Enabled: m.Enabled.ValueBool(), Scopes: []string{}, Audiences: []string{},
	}
	diags.Append(m.Scopes.ElementsAs(ctx, &client.Scopes, false)...)
	diags.Append(m.Audiences.ElementsAs(ctx, &client.Audiences, false)...)
	return client
}

func machineFromAPI(ctx context.Context, c iamlogin.MachineClient, m *machineClientModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(c.ID)
	m.Name = types.StringValue(c.Name)
	m.SPIFFEID = types.StringValue(c.SPIFFEID)
	m.Enabled = types.BoolValue(c.Enabled)
	var d diag.Diagnostics
	m.Scopes, d = types.SetValueFrom(ctx, types.StringType, nonNil(c.Scopes))
	diags.Append(d...)
	m.Audiences, d = types.SetValueFrom(ctx, types.StringType, nonNil(c.Audiences))
	diags.Append(d...)
}
