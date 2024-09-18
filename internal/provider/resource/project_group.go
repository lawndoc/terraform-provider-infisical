package resource

import (
	"context"
	"fmt"
	infisical "terraform-provider-infisical/internal/client"
	infisicalclient "terraform-provider-infisical/internal/client"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource = &ProjectGroupResource{}
)

// NewProjectResource is a helper function to simplify the provider implementation.
func NewProjectGroupResource() resource.Resource {
	return &ProjectGroupResource{}
}

// ProjectGroupResource is the resource implementation.
type ProjectGroupResource struct {
	client *infisical.Client
}

// projectResourceSourceModel describes the data source data model.
type ProjectGroupResourceModel struct {
	ProjectID    types.String       `tfsdk:"project_id"`
	ProjectSlug  types.String       `tfsdk:"project_slug"`
	GroupSlug    types.String       `tfsdk:"group_slug"`
	Roles        []ProjectGroupRole `tfsdk:"roles"`
	MembershipID types.String       `tfsdk:"membership_id"`
}

type ProjectGroupRole struct {
	RoleSlug                 types.String `tfsdk:"role_slug"`
	IsTemporary              types.Bool   `tfsdk:"is_temporary"`
	TemporaryRange           types.String `tfsdk:"temporary_range"`
	TemporaryAccessStartTime types.String `tfsdk:"temporary_access_start_time"`
}

// Metadata returns the resource type name.
func (r *ProjectGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project_group"
}

// Schema defines the schema for the resource.
func (r *ProjectGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Create project groups & save to Infisical. Only Machine Identity authentication is supported for this data source",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Description: "The id of the project.",
				Required:    true,
			},
			"project_slug": schema.StringAttribute{
				Description:   "The slug of the project.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"group_slug": schema.StringAttribute{
				Description: "The slug of the group.",
				Required:    true,
			},
			"membership_id": schema.StringAttribute{
				Description:   "The membership Id of the project group",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"roles": schema.SetNestedAttribute{
				Required:    true,
				Description: "The roles assigned to the project group",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"role_slug": schema.StringAttribute{
							Description: "The slug of the role",
							Required:    true,
						},
						"is_temporary": schema.BoolAttribute{
							Description: "Flag to indicate the assigned role is temporary or not. When is_temporary is true fields temporary_mode, temporary_range and temporary_access_start_time is required.",
							Optional:    true,
						},
						"temporary_range": schema.StringAttribute{
							Description: "TTL for the temporary time. Eg: 1m, 1h, 1d. Default: 1h",
							Optional:    true,
						},
						"temporary_access_start_time": schema.StringAttribute{
							Description: "ISO time for which temporary access should begin. The current time is used by default.",
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *ProjectGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*infisical.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Source Configure Type",
			fmt.Sprintf("Expected *http.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

// Create creates the resource and sets the initial Terraform state.
func (r *ProjectGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if !r.client.Config.IsMachineIdentityAuth {
		resp.Diagnostics.AddError(
			"Unable to create project group",
			"Only Machine Identity authentication is supported for this operation",
		)
		return
	}

	// Retrieve values from plan
	var plan ProjectGroupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var roles []infisical.CreateProjectGroupRequestRoles
	var hasAtleastOnePermanentRole bool
	for _, el := range plan.Roles {
		isTemporary := el.IsTemporary.ValueBool()
		temporaryRange := el.TemporaryRange.ValueString()
		TemporaryAccessStartTime := time.Now().UTC()

		if !isTemporary {
			hasAtleastOnePermanentRole = true
		}

		if el.TemporaryAccessStartTime.ValueString() != "" {
			var err error
			TemporaryAccessStartTime, err = time.Parse(time.RFC3339, el.TemporaryAccessStartTime.ValueString())
			if err != nil {
				resp.Diagnostics.AddError(
					"Error parsing field TemporaryAccessStartTime",
					fmt.Sprintf("Must provider valid ISO timestamp for field TemporaryAccessStartTime %s, role %s", el.TemporaryAccessStartTime.ValueString(), el.RoleSlug.ValueString()),
				)
				return
			}
		}

		// default values
		temporaryMode := ""
		if isTemporary {
			temporaryMode = TEMPORARY_MODE_RELATIVE
		}
		if isTemporary && temporaryRange == "" {
			temporaryRange = TEMPORARY_RANGE_DEFAULT
		}

		roles = append(roles, infisical.CreateProjectGroupRequestRoles{
			Role:                     el.RoleSlug.ValueString(),
			IsTemporary:              isTemporary,
			TemporaryMode:            temporaryMode,
			TemporaryRange:           temporaryRange,
			TemporaryAccessStartTime: TemporaryAccessStartTime,
		})
	}

	if !hasAtleastOnePermanentRole {
		resp.Diagnostics.AddError("Error assigning role to group", "Must have atleast one permanent role")
		return
	}

	projectDetail, err := r.client.GetProjectById(infisical.GetProjectByIdRequest{
		ID: plan.ProjectID.ValueString(),
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Error attaching group to project",
			"Couldn't fetch project details, unexpected error: "+err.Error(),
		)
		return
	}

	projectGroupResponse, err := r.client.CreateProjectGroup(infisical.CreateProjectGroupRequest{
		ProjectSlug: projectDetail.Slug,
		GroupSlug:   plan.GroupSlug.ValueString(),
		Roles:       roles,
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Error attaching group to project",
			"Couldn't create project group to Infiscial, unexpected error: "+err.Error(),
		)
		return
	}

	plan.ProjectSlug = types.StringValue(projectDetail.Slug)
	plan.MembershipID = types.StringValue(projectGroupResponse.Membership.ID)

	if err != nil {
		resp.Diagnostics.AddError(
			"Error fetching group details",
			"Couldn't find group in project, unexpected error: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read refreshes the Terraform state with the latest data.
func (r *ProjectGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if !r.client.Config.IsMachineIdentityAuth {
		resp.Diagnostics.AddError(
			"Unable to read project group",
			"Only Machine Identity authentication is supported for this operation",
		)
		return
	}

	// Get current state
	var state ProjectGroupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectGroupMembership, err := r.client.GetProjectGroupMembership(infisical.GetProjectGroupMembershipRequest{
		ProjectSlug: state.ProjectSlug.ValueString(),
		GroupSlug:   state.GroupSlug.ValueString(),
	})
	if err != nil {
		if err == infisicalclient.ErrNotFound {
			resp.State.RemoveResource(ctx)
			return
		} else {
			resp.Diagnostics.AddError(
				"Error reading project group membership",
				"Couldn't read project group membership from Infiscial, unexpected error: "+err.Error(),
			)
			return
		}
	}

	stateRoleMap := make(map[string]ProjectGroupRole)
	for _, role := range state.Roles {
		stateRoleMap[role.RoleSlug.ValueString()] = role
	}

	planRoles := make([]ProjectGroupRole, 0, len(projectGroupMembership.Membership.Roles))
	for _, el := range projectGroupMembership.Membership.Roles {
		val := ProjectGroupRole{
			RoleSlug:                 types.StringValue(el.Role),
			TemporaryRange:           types.StringValue(el.TemporaryRange),
			IsTemporary:              types.BoolValue(el.IsTemporary),
			TemporaryAccessStartTime: types.StringValue(el.TemporaryAccessStartTime.Format(time.RFC3339)),
		}

		if el.CustomRoleId != "" {
			val.RoleSlug = types.StringValue(el.CustomRoleSlug)
		}

		/*
			We do the following because we want to maintain the state when the API returns these properties
			with default values. We cannot use the computed property because it breaks the Set. Thus we
			handle this manually.
		*/
		previousRoleState, ok := stateRoleMap[val.RoleSlug.ValueString()]
		if ok {
			if previousRoleState.IsTemporary.ValueBool() && el.IsTemporary {
				if previousRoleState.TemporaryRange.IsNull() && el.TemporaryRange == TEMPORARY_RANGE_DEFAULT {
					val.TemporaryRange = types.StringNull()
				}
			}

			if previousRoleState.IsTemporary.IsNull() && !el.IsTemporary {
				val.IsTemporary = types.BoolNull()
			}
		}

		if !el.IsTemporary {
			val.TemporaryRange = types.StringNull()
			val.TemporaryAccessStartTime = types.StringNull()
		}

		planRoles = append(planRoles, val)
	}

	state.Roles = planRoles
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *ProjectGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if !r.client.Config.IsMachineIdentityAuth {
		resp.Diagnostics.AddError(
			"Unable to update project group",
			"Only Machine Identity authentication is supported for this operation",
		)
		return
	}

	// Retrieve values from plan
	var plan ProjectGroupResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ProjectGroupResourceModel
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.GroupSlug != state.GroupSlug {
		resp.Diagnostics.AddError(
			"Unable to update project group",
			fmt.Sprintf("Cannot change group slug, previous group: %s, new group: %s", state.GroupSlug, plan.GroupSlug),
		)
		return
	}

	var roles []infisical.UpdateProjectGroupRequestRoles
	var hasAtleastOnePermanentRole bool
	for _, el := range plan.Roles {
		isTemporary := el.IsTemporary.ValueBool()
		temporaryRange := el.TemporaryRange.ValueString()
		TemporaryAccessStartTime := time.Now().UTC()

		if !isTemporary {
			hasAtleastOnePermanentRole = true
		}

		if el.TemporaryAccessStartTime.ValueString() != "" {
			var err error
			TemporaryAccessStartTime, err = time.Parse(time.RFC3339, el.TemporaryAccessStartTime.ValueString())
			if err != nil {
				resp.Diagnostics.AddError(
					"Error parsing field TemporaryAccessStartTime",
					fmt.Sprintf("Must provider valid ISO timestamp for field TemporaryAccessStartTime %s, role %s", el.TemporaryAccessStartTime.ValueString(), el.RoleSlug.ValueString()),
				)
				return
			}
		}

		// default values
		temporaryMode := ""
		if isTemporary {
			temporaryMode = TEMPORARY_MODE_RELATIVE
		}
		if isTemporary && temporaryRange == "" {
			temporaryRange = "1h"
		}

		roles = append(roles, infisical.UpdateProjectGroupRequestRoles{
			Role:                     el.RoleSlug.ValueString(),
			IsTemporary:              isTemporary,
			TemporaryMode:            temporaryMode,
			TemporaryRange:           temporaryRange,
			TemporaryAccessStartTime: TemporaryAccessStartTime,
		})
	}

	if !hasAtleastOnePermanentRole {
		resp.Diagnostics.AddError("Error assigning role to group", "Must have atleast one permanent role")
		return
	}

	_, err := r.client.UpdateProjectGroup(infisical.UpdateProjectGroupRequest{
		ProjectSlug: state.ProjectSlug.ValueString(),
		GroupSlug:   plan.GroupSlug.ValueString(),
		Roles:       roles,
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Error assigning roles to group",
			"Couldn't update role, unexpected error: "+err.Error(),
		)
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *ProjectGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if !r.client.Config.IsMachineIdentityAuth {
		resp.Diagnostics.AddError(
			"Unable to delete project group",
			"Only Machine Identity authentication is supported for this operation",
		)
		return
	}

	var state ProjectGroupResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.DeleteProjectGroup(infisical.DeleteProjectGroupRequest{
		ProjectSlug: state.ProjectSlug.ValueString(),
		GroupSlug:   state.GroupSlug.ValueString(),
	})

	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting project group",
			"Couldn't delete project group from Infiscial, unexpected error: "+err.Error(),
		)
	}
}
