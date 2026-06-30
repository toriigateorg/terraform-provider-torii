package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/toriigateorg/torii/terraform-provider/internal/client"
)

type apiUserResource struct {
	c *client.Client
}

func NewAPIUserResource() resource.Resource { return &apiUserResource{} }

type apiUserModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	ExpiresAt   types.String `tfsdk:"expires_at"`
	Prefix      types.String `tfsdk:"prefix"`
	Disabled    types.Bool   `tfsdk:"disabled"`
	CreatedAt   types.String `tfsdk:"created_at"`
	Token       types.String `tfsdk:"token"`
}

func (r *apiUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_user"
}

func (r *apiUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A torii Service API user: a passwordless machine identity that authenticates to " +
			"services behind torii with a bearer token (torii_sat_...), bypassing SSO. Grant it access " +
			"to services by assigning roles via torii_api_user_role. The token is only available at " +
			"create time; to rotate it, replace the resource (e.g. terraform apply -replace).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:      true,
				Description:   "Unique identifier for the service user (1-200 chars).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Free-text description.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"expires_at": schema.StringAttribute{
				Optional: true,
				Description: "Optional RFC3339 expiry (use UTC, e.g. 2027-01-01T00:00:00Z). " +
					"After this time the token is rejected. Omit for no expiry.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"prefix": schema.StringAttribute{
				Computed:      true,
				Description:   "Short, non-secret display prefix of the token.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"disabled": schema.BoolAttribute{
				Computed:      true,
				Description:   "Whether the service user is disabled (rejected regardless of roles).",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"token": schema.StringAttribute{
				Computed:   true,
				Sensitive:  true,
				Description: "The plaintext bearer token (torii_sat_...). Pass it as `Authorization: Bearer` " +
					"or in the `X-Torii-Service-Token` header. Only known after create; the API never " +
					"returns it again.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *apiUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.c = c
}

// setAPIUserState copies server-owned fields onto the model. It deliberately
// leaves expires_at untouched (preserved from plan/state to avoid drift from
// server-side timestamp normalization) and sets token from the response, which
// is only non-empty on create.
func setAPIUserState(m *apiUserModel, u *client.APIUser) {
	m.ID = types.StringValue(u.ID)
	m.Name = types.StringValue(u.Name)
	m.Description = types.StringValue(u.Description)
	m.Prefix = types.StringValue(u.Prefix)
	m.Disabled = types.BoolValue(u.Disabled)
	m.CreatedAt = types.StringValue(u.CreatedAt)
	m.Token = types.StringValue(u.Token)
}

func (r *apiUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apiUserModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := client.APIUserCreate{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}
	if !plan.ExpiresAt.IsNull() && !plan.ExpiresAt.IsUnknown() && plan.ExpiresAt.ValueString() != "" {
		v := plan.ExpiresAt.ValueString()
		in.ExpiresAt = &v
	}

	out, err := r.c.CreateAPIUser(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("create api user failed", err.Error())
		return
	}

	expiresAt := plan.ExpiresAt
	setAPIUserState(&plan, out)
	plan.ExpiresAt = expiresAt
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apiUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	out, err := r.c.GetAPIUser(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("read api user failed", err.Error())
		return
	}

	// token and expires_at are not returned by the GET endpoint; preserve them
	// from prior state so they don't churn or drift.
	token := state.Token
	expiresAt := state.ExpiresAt
	setAPIUserState(&state, out)
	state.Token = token
	state.ExpiresAt = expiresAt
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *apiUserResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("update not supported", "torii_api_user has no mutable attributes; all inputs force replacement")
}

func (r *apiUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apiUserModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.DeleteAPIUser(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("delete api user failed", err.Error())
	}
}

func (r *apiUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
