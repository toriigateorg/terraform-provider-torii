# A Service API user: a passwordless machine identity that reaches services
# behind torii with a bearer token, bypassing SSO. Grant it access by assigning
# roles (the roles must already cover the target services).
resource "torii_api_user" "ci_deploy" {
  name        = "ci-deploy-bot"
  description = "Used by the prod deploy pipeline"

  # Optional. Omit for a token that never expires. Use UTC (Z).
  expires_at = "2027-01-01T00:00:00Z"
}

resource "torii_api_user_role" "ci_deploy_deployer" {
  api_user_id = torii_api_user.ci_deploy.id
  role_id     = torii_role.deployer.id
}

# The token is sensitive and only known after create. Surface it through an
# output (mark sensitive) or feed it straight into the consumer that needs it.
output "ci_deploy_token" {
  value     = torii_api_user.ci_deploy.token
  sensitive = true
}
