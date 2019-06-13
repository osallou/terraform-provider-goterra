package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

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
			"timeout": {
				Type:     schema.TypeInt,
				Optional: true,
				Default:  30,
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

	address := meta.(ProviderConfig).Address
	if d.Get("address").(string) != "" {
		address = d.Get("address").(string)
	}

	remote := []string{address, "store", d.Get("deployment").(string), d.Get("key").(string)}
	limit := time.Now().Add(time.Duration(d.Get("timeout").(int)) * time.Second)
	current := time.Now()
	for !current.After(limit) {
		current = time.Now()
		req, _ := http.NewRequest("GET", strings.Join(remote, "/"), nil)
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", d.Get("token").(string)))
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to contact server %s, %s", d.Get("address"), err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == 403 {
			log.Printf("[INFO] failed to get key, unauthorized\n")
			return fmt.Errorf("failed to get key, unauthorized")
		}
		if resp.StatusCode != 200 {
			log.Printf("[INFO] failed to get key %s, waiting\n", d.Get("key").(string))
			time.Sleep(1 * time.Second)
			//d.Set("data", "NotFound")
			//d.SetId(d.Get("key").(string))
			//return nil
			continue
		}
		respData := &KeyValue{}
		json.NewDecoder(resp.Body).Decode(respData)
		log.Printf("[DEBUG] deployment %+v", respData)
		d.SetId(d.Get("key").(string))
		d.Set("data", respData.Value)
		return nil
	}
	d.SetId(d.Get("key").(string))
	d.Set("data", "NotFound")
	return nil
}
