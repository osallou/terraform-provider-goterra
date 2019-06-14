package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"
)

func resourcePush() *schema.Resource {
	return &schema.Resource{
		Create: resourcePushCreate,
		Read:   resourcePushRead,
		Update: resourcePushUpdate,
		Delete: resourcePushDelete,

		Schema: map[string]*schema.Schema{
			"address": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"token": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"deployment": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"key": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"value": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func resourcePushCreate(d *schema.ResourceData, m interface{}) error {
	address := d.Get("address").(string)
	options := PushOptions{}
	options.url = m.(ProviderConfig).Address
	if address != "" {
		options.url = address
	}
	options.deployment = d.Get("deployment").(string)
	options.token = d.Get("token").(string)
	options.key = d.Get("key").(string)
	options.value = d.Get("value").(string)
	createPush(options)
	d.SetId(fmt.Sprintf("%s-%s", options.deployment, options.key))
	log.Printf("[INFO] Pushed a: %s\n", options.key)
	return resourceServerRead(d, m)
}

func resourcePushRead(d *schema.ResourceData, m interface{}) error {
	return nil
}

func resourcePushUpdate(d *schema.ResourceData, m interface{}) error {
	return resourceServerRead(d, m)
}

func resourcePushDelete(d *schema.ResourceData, m interface{}) error {
	return nil
}

// PushOptions to connect to goterra and set a key/value
type PushOptions struct {
	url        string
	key        string
	value      string
	token      string
	deployment string
}

// DeploymentData represents data sent to update a deployment value
type DeploymentData struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func createPush(options PushOptions) bool {
	client := &http.Client{}
	remote := []string{options.url, "store", options.deployment}
	deploymentData := DeploymentData{Key: options.key, Value: options.value}
	byteData, _ := json.Marshal(&deploymentData)
	req, _ := http.NewRequest("PUT", strings.Join(remote, "/"), bytes.NewBuffer(byteData))
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", options.token))
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("failed to contact server %s\n", options.url)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("failed to create deployment %d\n", resp.StatusCode)
		return false
	}
	respData := &Deployment{}
	json.NewDecoder(resp.Body).Decode(respData)
	log.Printf("[DEBUG] deployment %+v", respData)
	return true
}
