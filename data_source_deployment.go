package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
)

func dataSourceDeployment() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceDeploymentRead,

		Schema: map[string]*schema.Schema{
			"key": {
				Type:     schema.TypeString,
				Required: true,
			},
			"address": {
				Type:     schema.TypeString,
				Required: true,
			},
			"deployment": {
				Type:     schema.TypeString,
				Required: true,
			},
			"token": {
				Type:     schema.TypeString,
				Required: true,
			},
			"data": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

// KeyValue represents a key value response when fetching key from goterra
type KeyValue struct {
	Key   string
	Value string
}

func dataSourceDeploymentRead(d *schema.ResourceData, meta interface{}) error {
	client := &http.Client{}
	remote := []string{d.Get("address").(string), "deployment", d.Get("deployment").(string), d.Get("key").(string)}
	req, _ := http.NewRequest("GET", strings.Join(remote, "/"), nil)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.Get("token").(string)))
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to contact server %s", d.Get("address"))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("[INFO] failed to get deployment key\n")
		d.Set("data", "NotFound")
		d.SetId(d.Get("key").(string))
		return nil
	}
	respData := &KeyValue{}
	json.NewDecoder(resp.Body).Decode(respData)
	log.Printf("[DEBUG] deployment %+v", respData)
	d.Set("data", respData.Value)
	d.SetId(d.Get("key").(string))
	return nil
}
