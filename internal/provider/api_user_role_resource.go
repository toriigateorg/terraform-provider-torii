package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/toriigateorg/torii/terraform-provider/internal/client"
)

type apiUserRoleResource struct {
	c *client.Client
}

func NewAPIUserRoleResource() resource.Resource { return &apiUserRoleResource{} }

type apiUserRoleModel struct {
	ID        types.String `tfsdk:"id"`
	APIUserID types.String `tfsdk:"api_user_id"`
	RoleID    types.String `tfsdk:"role_id"`
}

func (r *apiUserRoleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_user_role"
}

func (r *apiUserRoleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "Assigns a role to a Service API user, granting it access to the services that role " +
			"covers. The composite ID is `<api_user_id>:<role_id>`. The built-in `all` role is auto-" +
			"assigned by torii and cannot be managed by this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"api_user_id": schema.StringAttribute{Required: true, PlanModifiers: replace},
			"role_id":     schema.StringAttribute{Required: true, PlanModifiers: replace},
		},
	}
}

func (r *apiUserRoleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *apiUserRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan apiUserRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.AssignAPIUserRole(ctx, plan.APIUserID.ValueString(), plan.RoleID.ValueString()); err != nil {
		resp.Diagnostics.AddError("assign api user role failed", err.Error())
		return
	}
	plan.ID = types.StringValue(compositeID(plan.APIUserID.ValueString(), plan.RoleID.ValueString()))
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiUserRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state apiUserRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	apiUserID := state.APIUserID.ValueString()
	roleID := state.RoleID.ValueString()
	roles, err := r.c.ListAPIUserRoles(ctx, apiUserID)
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("list api user roles failed", err.Error())
		return
	}
	found := false
	for _, role := range roles {
		if role.ID == roleID {
			found = true
			break
		}
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = types.StringValue(compositeID(apiUserID, roleID))
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *apiUserRoleResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("update not supported", "torii_api_user_role has no mutable attributes; both fields force replacement")
}

func (r *apiUserRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state apiUserRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.RevokeAPIUserRole(ctx, state.APIUserID.ValueString(), state.RoleID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("revoke api user role failed", err.Error())
	}
}

func (r *apiUserRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("invalid import id", "expected format <api_user_id>:<role_id>")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("api_user_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
