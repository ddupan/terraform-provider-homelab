package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"git.ddupan.top/panxiao81/terraform-provider-homelab/internal/iamlogin"
)

func newOIDCClientResource() resource.Resource { return &oidcClientResource{} }

type oidcClientResource struct{ api *iamlogin.Client }

type oidcClientModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	RedirectURIs           types.List   `tfsdk:"redirect_uris"`
	Scopes                 types.Set    `tfsdk:"scopes"`
	PostLogoutRedirectURIs types.List   `tfsdk:"post_logout_redirect_uris"`
	LoginEnabled           types.Bool   `tfsdk:"login_enabled"`
	PKCERequired           types.Bool   `tfsdk:"pkce_required"`
	SecretRotation         types.String `tfsdk:"secret_rotation"`
	Secret                 types.String `tfsdk:"secret"`
}

func (r *oidcClientResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oidc_client"
}

func (r *oidcClientResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A confidential authorization-code OIDC client of iam-login (client_secret_basic).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required: true, Description: "Client ID. Changing it replaces the client.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name":          schema.StringAttribute{Required: true},
			"redirect_uris": schema.ListAttribute{Required: true, ElementType: types.StringType},
			"scopes": schema.SetAttribute{
				Required: true, ElementType: types.StringType,
				Description: "Must include openid. offline_access also grants refresh tokens. Scopes are delegation, not permission.",
			},
			"post_logout_redirect_uris": schema.ListAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Default: listdefault.StaticValue(types.ListValueMust(types.StringType, nil)),
			},
			"login_enabled": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "false refuses new authorizations and refreshes for the client.",
			},
			"pkce_required": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "Turn off only for confidential clients that cannot send PKCE (e.g. Gitea 1.27).",
			},
			"secret_rotation": schema.StringAttribute{
				Optional:    true,
				Description: "Any value; changing it rotates the secret (the old one stops working immediately).",
			},
			"secret": schema.StringAttribute{
				Computed: true, Sensitive: true,
				Description: "Client secret, known only from creation or rotation; null after import until rotated.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *oidcClientResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	configured, ok := req.ProviderData.(clients)
	if !ok || configured.iamLogin == nil {
		resp.Diagnostics.AddError("iam_login is not configured", "homelab_oidc_client needs the provider's iam_login block.")
		return
	}
	r.api = configured.iamLogin
}

func (r *oidcClientResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan oidcClientModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	client := r.toAPI(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	created, secret, err := r.api.Create(ctx, client)
	if err != nil {
		resp.Diagnostics.AddError("Creating OIDC client "+client.ID, err.Error())
		return
	}
	r.fromAPI(ctx, created, &plan, &resp.Diagnostics)
	plan.Secret = types.StringValue(secret)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *oidcClientResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state oidcClientModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.api.Get(ctx, state.ID.ValueString())
	if errors.Is(err, iamlogin.ErrNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Reading OIDC client "+state.ID.ValueString(), err.Error())
		return
	}
	r.fromAPI(ctx, current, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *oidcClientResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state oidcClientModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	client := r.toAPI(ctx, plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.api.Update(ctx, client)
	if err != nil {
		resp.Diagnostics.AddError("Updating OIDC client "+client.ID, err.Error())
		return
	}
	r.fromAPI(ctx, updated, &plan, &resp.Diagnostics)
	plan.Secret = state.Secret
	if !plan.SecretRotation.Equal(state.SecretRotation) {
		secret, err := r.api.RotateSecret(ctx, client.ID)
		if err != nil {
			resp.Diagnostics.AddError("Rotating the secret of OIDC client "+client.ID, err.Error())
			return
		}
		plan.Secret = types.StringValue(secret)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *oidcClientResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state oidcClientModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.Delete(ctx, state.ID.ValueString()); err != nil && !errors.Is(err, iamlogin.ErrNotFound) {
		resp.Diagnostics.AddError("Deleting OIDC client "+state.ID.ValueString(), err.Error())
	}
}

// ModifyPlan marks the secret unknown when secret_rotation changes. Without it the secret's
// UseStateForUnknown plans the old value, and the rotated one returned by apply makes Terraform
// fail with "Provider produced inconsistent result after apply".
func (r *oidcClientResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return // create or destroy
	}
	var planned, prior types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("secret_rotation"), &planned)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("secret_rotation"), &prior)...)
	if resp.Diagnostics.HasError() || planned.Equal(prior) {
		return
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("secret"), types.StringUnknown())...)
}

// ImportState adopts an existing client by ID. Its secret cannot be read back: set secret_rotation to obtain one.
func (r *oidcClientResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
