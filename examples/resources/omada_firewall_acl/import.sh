# Import by rule ID (the ACL type — gateway/switch/eap — is detected automatically):
terraform import omada_firewall_acl.mgmt 692c13f675ee724076c80d44

# Or scope to a named site:
terraform import omada_firewall_acl.mgmt 'Home/692c13f675ee724076c80d44'
