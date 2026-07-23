# Discover firewall ACL IDs and types (e.g. to write import blocks).
data "omada_firewall_acls" "all" {}

output "acl_ids" {
  value = { for a in data.omada_firewall_acls.all.acls : a.name => "${a.type}/${a.id}" }
}
