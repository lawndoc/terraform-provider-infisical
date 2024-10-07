---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "infisical_access_approval_policy Resource - terraform-provider-infisical"
subcategory: ""
description: |-
  Create access approval policy for your projects
---

# infisical_access_approval_policy (Resource)

Create access approval policy for your projects

## Example Usage

```terraform
terraform {
  required_providers {
    infisical = {
      # version = <latest version>
      source = "infisical/infisical"
    }
  }
}

provider "infisical" {
  host          = "https://app.infisical.com" # Only required if using self hosted instance of Infisical, default is https://app.infisical.com
  client_id     = "<>"
  client_secret = "<>"
}

resource "infisical_project" "example" {
  name = "example"
  slug = "example"
}

resource "infisical_access_approval_policy" "prod-policy" {
  project_id       = infisical_project.example.id
  name             = "my-approval-policy"
  environment_slug = "prod"
  secret_path      = "/"
  approvers = [
    {
      type = "group"
      id   = "52c70c28-9504-4b88-b5af-ca2495dd277d"
    },
    {
      type     = "user"
      username = "name@infisical.com"
  }]
  required_approvals = 1
  enforcement_level  = "soft"
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `approvers` (Attributes Set) The required approvers (see [below for nested schema](#nestedatt--approvers))
- `environment_slug` (String) The environment to apply the access approval policy to
- `project_id` (String) The ID of the project to add the access approval policy
- `required_approvals` (Number) The number of required approvers
- `secret_path` (String) The secret path to apply the access approval policy to

### Optional

- `enforcement_level` (String) The enforcement level of the policy. This can either be hard or soft
- `name` (String) The name of the access approval policy

### Read-Only

- `id` (String) The ID of the access approval policy

<a id="nestedatt--approvers"></a>
### Nested Schema for `approvers`

Required:

- `type` (String) The type of approver. Either group or user

Optional:

- `id` (String) The ID of the approver
- `username` (String) The username of the approver. By default, this is the email