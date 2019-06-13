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

func resourceDeployment() *schema.Resource {
	return &schema.Resource{
		Create: resourceServerCreate,
		Read:   resourceServerRead,
		Update: resourceServerUpdate,
		Delete: resourceServerDelete,

		Schema: map[string]*schema.Schema{
			"address": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"apikey": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				Default:  "",
			},
			"token": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceServerCreate(d *schema.ResourceData, m interface{}) error {
	address := d.Get("address").(string)
	apikey := d.Get("apikey").(string)
	options := Options{}
	options.url = m.(ProviderConfig).Address
	options.token = m.(ProviderConfig).APIKey
	if address != "" {
		options.url = address
	}
	if apikey != "" {
		options.token = apikey
	}
	deployment := createDeployment(options)
	if deployment == nil {
		return fmt.Errorf("Failed to create deployment")
	}
	d.SetId(deployment.ID)
	d.Set("token", deployment.Token)
	log.Printf("[INFO] Created a goterra deployment: %+v\n", deployment)
	return resourceServerRead(d, m)
}

func resourceServerRead(d *schema.ResourceData, m interface{}) error {
	return nil
}

func resourceServerUpdate(d *schema.ResourceData, m interface{}) error {
	return resourceServerRead(d, m)
}

func resourceServerDelete(d *schema.ResourceData, m interface{}) error {
	address := d.Get("address").(string)
	options := Options{}
	options.deployment = d.Id()
	options.token = d.Get("token").(string)
	options.url = m.(ProviderConfig).Address
	if address != "" {
		options.url = address
	}
	deleteDeployment(options)
	return nil
}

// Options to connect to goterra
type Options struct {
	url        string
	deployment string
	token      string
}

// Deployment gets info of a new deployment
type Deployment struct {
	URL   string `json:"url"`
	ID    string `json:"id"`
	Token string `json:"token"`
}

func createDeployment(options Options) *Deployment {
	client := &http.Client{}
	remote := []string{options.url, "store"}
	byteData := make([]byte, 0)
	req, _ := http.NewRequest("POST", strings.Join(remote, "/"), bytes.NewReader(byteData))
	req.Header.Add("X-API-Key", options.token)
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("failed to contact server %s\n", options.url)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("failed to create deployment %d\n", resp.StatusCode)
		return nil
	}
	respData := &Deployment{}
	json.NewDecoder(resp.Body).Decode(respData)
	log.Printf("[DEBUG] deployment %+v", respData)
	return respData
}

func deleteDeployment(options Options) bool {
	client := &http.Client{}
	remote := []string{options.url, "store", options.deployment}
	byteData := make([]byte, 0)
	req, _ := http.NewRequest("DELETE", strings.Join(remote, "/"), bytes.NewReader(byteData))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", options.token))
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("failed to contact server %s\n", options.url)
		return true
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("failed to create deployment %d\n", resp.StatusCode)
		return true
	}
	log.Printf("[INFO] Deployment deleted")
	return false
}
