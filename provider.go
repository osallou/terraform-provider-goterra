package main

import (
	"fmt"

	"github.com/hashicorp/terraform/helper/schema"
)

// ProviderConfig is the provider base configuration
type ProviderConfig struct {
	Address string
	APIKey  string
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	address := d.Get("address").(string)
	apikey := d.Get("apikey").(string)
	if address == "" || apikey == "" {
		return nil, fmt.Errorf("address or apikey are not defined")
	}
	config := ProviderConfig{Address: d.Get("address").(string), APIKey: d.Get("apikey").(string)}
	return config, nil
}

// Provider is a Terraform provider to manage goterra resources
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"address": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Address of goterra",
			},
			"apikey": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "User API Key",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"goterra_deployment":  resourceDeployment(),
			"goterra_push":        resourcePush(),
			"goterra_application": resourceApplication(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"goterra_deployment": dataSourceDeployment(),
		},
		ConfigureFunc: providerConfigure,
	}
}
