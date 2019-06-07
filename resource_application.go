package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const goterraTmplPre string = `#!/bin/bash
set -e

ON_ERROR () {
	trap ERR
	if [ -e /opt/got/got.log ]; then
		/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token ${GOT_TOKEN} put _log @/opt/got/got.log
	fi
	/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token ${GOT_TOKEN} put status failed
	exit 1
}

trap ON_ERROR ERR

export GOT_TRIM=1000000

echo "Set up goterra"

if [ -n "$(command -v yum)" ]; then
    yum -y install curl
fi

if [ -n "$(command -v apt)" ]; then
    export DEBIAN_NONINTERACTIVE=1
    apt-get update && apt-get install -y  curl
fi

get_latest_release() {
	curl --silent "https://api.github.com/repos/osallou/goterra/releases/latest" |
	  grep '"tag_name":' |
	  sed -E 's/.*"([^"]+)".*/\1/'
}
cliversion=` + "`get_latest_release`" + `

echo "[INFO] initialization"
mkdir -p /opt/got
curl -L -o /opt/got/goterra-cli https://github.com/osallou/goterra/releases/download/$cliversion/goterra-cli.linux.amd64
chmod +x /opt/got/goterra-cli

/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token ${GOT_TOKEN} put status start
`
const goterraTmpPost string = `
echo "[TODO] send log with put log @/opt/got/got.log"
echo "[INFO] setup is over"
/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token ${GOT_TOKEN} put status over
if [ -e /opt/got/got.log ]; then
	/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token ${GOT_TOKEN} put _log @/opt/got/got.log
fi

`

func resourceApplication() *schema.Resource {
	return &schema.Resource{
		Create: resourceApplicationCreate,
		Read:   resourceApplicationRead,
		Update: resourceApplicationUpdate,
		Delete: resourceApplicationDelete,

		Schema: map[string]*schema.Schema{
			"address": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"apikey": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"deployment": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"deployment_token": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"deployment_address": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"application": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"namespace": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},
			"cloudinit": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceApplicationCreate(d *schema.ResourceData, m interface{}) error {
	address := d.Get("address").(string)
	options := ApplicationOptions{}
	options.url = address
	options.apikey = d.Get("apikey").(string)
	options.deployment = d.Get("deployment").(string)
	options.application = d.Get("application").(string)
	options.namespace = d.Get("namespace").(string)
	options.deploymentToken = d.Get("deployment_token").(string)
	options.deploymentAddress = d.Get("deployment_address").(string)
	if options.deploymentAddress == "" {
		options.deploymentAddress = address
	}
	cloudinit, err := createApp(options)
	if err != nil {
		return err
	}
	d.SetId(fmt.Sprintf("%s-%s", options.deployment, options.application))
	d.Set("cloudinit", cloudinit)
	if cloudinit == "" {
		log.Printf("[ERROR] failed to create cloudinit")
		return nil
	}
	log.Printf("[INFO] Cloudinit file: %s\n", cloudinit)
	return resourceServerRead(d, m)
}

func resourceApplicationRead(d *schema.ResourceData, m interface{}) error {
	return nil
}

func resourceApplicationUpdate(d *schema.ResourceData, m interface{}) error {
	return resourceServerRead(d, m)
}

func resourceApplicationDelete(d *schema.ResourceData, m interface{}) error {
	return nil
}

// DeployToken is deploy bind answer
type DeployToken struct {
	Token string `json:"token"`
}

func createApp(options ApplicationOptions) (string, error) {
	cloudinit := ""
	client := &http.Client{}
	remote := []string{options.url, "deploy", "session", "bind"}
	byteData := make([]byte, 0)
	req, _ := http.NewRequest("POST", strings.Join(remote, "/"), bytes.NewReader(byteData))
	req.Header.Add("X-API-Key", options.apikey)
	req.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("failed to contact server %s\n", options.url)
		return "", fmt.Errorf("[ERROR] failed to contact server %s", options.url)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("failed to bind %d\n", resp.StatusCode)
		return "", fmt.Errorf("[ERROR] failed to bind %d", resp.StatusCode)
	}
	respBind := &DeployToken{}
	json.NewDecoder(resp.Body).Decode(respBind)

	remote = []string{options.url, "deploy", "ns", options.namespace, "app", options.application}
	byteData = make([]byte, 0)
	req, _ = http.NewRequest("GET", strings.Join(remote, "/"), bytes.NewReader(byteData))
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", respBind.Token))
	req.Header.Add("Content-Type", "application/json")
	respApp, err := client.Do(req)
	if err != nil {
		log.Printf("failed to contact server %s\n", options.url)
		return "", fmt.Errorf("[ERROR] failed to contact server %s", options.url)
	}
	defer respApp.Body.Close()
	if respApp.StatusCode != 200 {
		log.Printf("failed to get app %d\n", respApp.StatusCode)
		return "", fmt.Errorf("[ERROR] failed to get app %d", respApp.StatusCode)
	}
	respAppInfo := &RespApplication{}
	decerr := json.NewDecoder(respApp.Body).Decode(respAppInfo)
	if decerr != nil {
		log.Printf("[ERROR] decode error %s", decerr)
		return "", fmt.Errorf("[ERROR] Decode error %s", decerr)
	}
	options.token = respBind.Token

	loadedScripts := make(map[string]bool)
	scripts := make([]string, 0)
	// Loop over app recipes, and go over parent recipes
	jr, _ := json.Marshal(respAppInfo)
	log.Printf("[INFO] app = %s", jr)

	scriptTxt := goterraTmplPre + "\n"

	for varName, varValue := range respAppInfo.App.Inputs {
		if strings.HasPrefix(varName, "env_") {
			exportName := strings.Replace(varName, "env_", "", 1)
			scriptTxt += fmt.Sprintf("export %s=%q\n", exportName, varValue)
		}
	}

	for _, appRecipe := range respAppInfo.App.Recipes {
		recipe, err := getRecipe(options, appRecipe)
		if err != nil {
			log.Printf("[ERROR] Failed to get recipe")
			return "", fmt.Errorf("[ERROR] failed to get recipe")
		}
		if _, ok := loadedScripts[recipe.Name]; ok {
			// Already loaded
		} else {
			loadedScripts[recipe.Name] = true
			scripts = append(scripts, recipe.Script)
		}
		if recipe.ParentRecipe != "" {
			parentRecipes, err := getParentRecipe(options, recipe.ParentRecipe)
			if err != nil {
				log.Printf("[ERROR] Failed to get recipe")
				return "", err
			}
			for _, parentRecipe := range parentRecipes {
				if _, ok := loadedScripts[parentRecipe.Name]; ok {
					// Already loaded
				} else {
					loadedScripts[parentRecipe.Name] = true
					scripts = append(scripts, parentRecipe.Script)
				}
			}
		}

		for i := len(scripts) - 1; i >= 0; i-- {
			scripts[i] = strings.Replace(scripts[i], "${GOT_ID}", options.application, -1)
			scripts[i] = strings.Replace(scripts[i], "${GOT_URL}", options.deploymentAddress, -1)
			scripts[i] = strings.Replace(scripts[i], "${GOT_TOKEN}", options.deploymentToken, -1)
			scripts[i] = strings.Replace(scripts[i], "${GOT_DEP}", options.deployment, -1)

			errRecipe := addRecipe(options, i, scripts[i])
			if errRecipe != nil {
				return "", errRecipe
			}
			recipeIndex := "_recipe" + fmt.Sprintf("%s_%d", options.application, i)
			scriptTxt += "\n" + "/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token ${GOT_TOKEN} get " + recipeIndex + " > /opt/got/" + recipeIndex + ".sh\n"
			scriptTxt += "chmod +x /opt/got/" + recipeIndex + ".sh\n"
			scriptTxt += "/opt/got/" + recipeIndex + ".sh &>> /opt/got/${GOT_ID}.log"
		}

		scripts = make([]string, 0)

	}

	scriptTxt = scriptTxt + "\n" + goterraTmpPost
	// write cloudinit file
	cloudinit = options.application + ".sh"

	scriptTxt = strings.Replace(scriptTxt, "${GOT_ID}", options.application, -1)
	scriptTxt = strings.Replace(scriptTxt, "${GOT_URL}", options.deploymentAddress, -1)
	scriptTxt = strings.Replace(scriptTxt, "${GOT_TOKEN}", options.deploymentToken, -1)
	scriptTxt = strings.Replace(scriptTxt, "${GOT_DEP}", options.deployment, -1)

	errFile := ioutil.WriteFile(cloudinit, []byte(scriptTxt), 0644)
	if errFile != nil {
		return "", fmt.Errorf("[ERROR] failed to write cloudinit file")
	}
	return cloudinit, nil
}

func addRecipe(options ApplicationOptions, index int, script string) error {
	remote := []string{options.deploymentAddress, "store", options.deployment}
	recipeData := DeploymentData{Key: "_recipe" + fmt.Sprintf("%s_%d", options.application, index), Value: script}
	byteData, _ := json.Marshal(recipeData)
	req, _ := http.NewRequest("PUT", strings.Join(remote, "/"), bytes.NewBuffer(byteData))
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", options.deploymentToken))
	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("failed to contact server %s\n", options.url)
		return fmt.Errorf("[ERROR] Failed to store recipe %d: %s", index, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("failed to create deployment %d\n", resp.StatusCode)
		return fmt.Errorf("[ERROR] Failed to store recipe %d: %d", index, resp.StatusCode)
	}
	return nil
}

func getParentRecipe(options ApplicationOptions, recipeID string) (recipes []Recipe, err error) {
	log.Printf("[INFO] load parent recipes of %s", recipeID)
	recipes = make([]Recipe, 0)
	parentRecipe, err := getRecipe(options, recipeID)
	if err != nil {
		return recipes, fmt.Errorf("failed to get recipe %s", recipeID)
	}
	recipes = append(recipes, *parentRecipe)
	hasParent := true
	count := 0
	for hasParent && count <= 1000 {
		log.Printf("[INFO] parent = %s", parentRecipe.ParentRecipe)
		if parentRecipe.ParentRecipe == "" {
			log.Printf("[INFO] no parent")
			hasParent = false
		} else {
			parentRecipe, err = getRecipe(options, parentRecipe.ParentRecipe)
			if err != nil {
				return recipes, fmt.Errorf("failed to get recipe %s", parentRecipe.ParentRecipe)
			}
			recipes = append(recipes, *parentRecipe)
		}
		count++
	}
	if count >= 1000 {
		return recipes, fmt.Errorf("it seems there is an infinite loop on recipes")
	}
	log.Printf("[INFO] Parent recipes %+v\n", recipes)
	return recipes, nil
}

func getRecipe(options ApplicationOptions, recipeID string) (recipe *Recipe, err error) {
	log.Printf("[INFO] load recipe %s", recipeID)
	remote := []string{options.url, "deploy", "ns", options.namespace, "recipe", recipeID}
	byteData := make([]byte, 0)
	req, _ := http.NewRequest("GET", strings.Join(remote, "/"), bytes.NewReader(byteData))
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", options.token))
	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("failed to contact server %s\n", options.url)
		return nil, fmt.Errorf("Failed to get recipe %s", recipeID)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("failed to create deployment %d\n", resp.StatusCode)
		return nil, fmt.Errorf("Failed to get recipe %s", recipeID)
	}
	resprecipe := &RespRecipe{}
	json.NewDecoder(resp.Body).Decode(resprecipe)
	log.Printf("[DEBUG] fetched recipe %s", resprecipe.Recipe.Name)
	recipe = &resprecipe.Recipe
	return recipe, nil
}

type RespRecipe struct {
	Recipe Recipe `json:"recipe"`
}

// Recipe describe a recipe for an app
type Recipe struct {
	ID           primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Script       string             `json:"script"`
	Public       bool               `json:"public"`
	Namespace    string             `json:"namespace"`
	BaseImage    string             `json:"base"`
	ParentRecipe string             `json:"parent"`
}

type RespApplication struct {
	App Application `json:"app"`
}

// Application descripe an app to deploy
type Application struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Public      bool               `json:"public"`
	Recipes     []string           `json:"recipes"`
	Namespace   string             `json:"namespace"`
	Templates   map[string]string  `json:"templates"`
	Inputs      map[string]string  `json:"inputs"` // expected inputs
}

// ApplicationOptions to connect to goterra and get recipes for app
type ApplicationOptions struct {
	url               string
	apikey            string
	deployment        string
	deploymentToken   string
	deploymentAddress string
	application       string
	namespace         string
	token             string
}
