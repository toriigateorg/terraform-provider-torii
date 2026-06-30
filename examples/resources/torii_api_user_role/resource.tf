# Assigns a role to a Service API user, granting it access to the services that
# role covers. The built-in "all" role is auto-assigned by torii and must not be
# managed here.
resource "torii_api_user_role" "ci_deploy_deployer" {
  api_user_id = torii_api_user.ci_deploy.id
  role_id     = torii_role.deployer.id
}
