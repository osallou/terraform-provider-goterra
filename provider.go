package main

import (
	"github.com/hashicorp/terraform/helper/schema"
)

// Provider is a Terraform provider to manage goterra resources
func Provider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"goterra_deployment": resourceDeployment(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"goterra_deployment": dataSourceDeployment(),
		},
	}
}
