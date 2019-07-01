package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform/helper/schema"

	terraModel "github.com/osallou/goterra-lib/lib/model"
)

const goterraTmplPre string = `#!/bin/bash
set -e

export TOKEN="${GOT_TOKEN}"

ON_ERROR () {
	trap ERR
	if [ -e /opt/got/${GOT_ID}.log ]; then
		/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token $TOKEN put _log_app_${GOT_NAME}_${HOSTNAME} @/opt/got/${GOT_ID}.log
	fi
	/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token $TOKEN put status_app_${GOT_NAME}_${HOSTNAME} failed
	exit 1
}

trap ON_ERROR ERR

export GOT_TRIM=1000000

echo "Set up goterra"

if [ -n "$(command -v yum)" ]; then
    yum -y install curl dos2unix
fi

if [ -n "$(command -v apt)" ]; then
	export DEBIAN_NONINTERACTIVE=1
	systemctl stop apt-daily.timer || true
	systemctl disable apt-daily.timer || true
	systemctl mask apt-daily.service || true
	systemctl daemon-reload
	apt-get purge -y unattended-upgrades || true
	time (while ps -opid= -C apt-get > /dev/null; do sleep 1; done); echo "Waiting for apt unlock"
    apt-get update && apt-get install -y  curl dos2unix
fi

get_latest_release() {
	curl --silent "https://api.github.com/repos/osallou/goterra-store/releases/latest" |
	  grep '"tag_name":' |
	  sed -E 's/.*"([^"]+)".*/\1/'
}
cliversion=` + "`get_latest_release`" + `

send_start_ts() {
	cur=` + "`date +%s`" + `
	/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token $TOKEN put _ts_start_${GOT_NAME}_${HOSTNAME} $cur
}

send_end_ts() {
	cur=` + "`date +%s`" + `
	/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token $TOKEN put _ts_end_${GOT_NAME}_${HOSTNAME} $cur
}

echo "[INFO] initialization"

mkdir -p /opt/got
curl -L -o /opt/got/goterra-cli https://github.com/osallou/goterra-store/releases/download/$cliversion/goterra-cli.linux.amd64
chmod +x /opt/got/goterra-cli
send_start_ts

/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token $TOKEN put status_app_${GOT_NAME}_${HOSTNAME} start
`
const goterraTmpPost string = `
echo "[INFO] setup is over"
send_end_ts
/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token $TOKEN put status_app_${GOT_NAME}_${HOSTNAME} over
if [ -e /opt/got/${GOT_ID}.log ]; then
	/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token $TOKEN put _log_app_${GOT_NAME}_${HOSTNAME} @/opt/got/${GOT_ID}.log
fi

`

func resourceApplication() *schema.Resource {
	return &schema.Resource{
		Create: resourceApplicationCreate,
		Read:   resourceApplicationRead,
		Update: resourceApplicationUpdate,
		Delete: resourceApplicationDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"address": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"apikey": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
			},
			"recipe_tags": &schema.Schema{
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional: true,
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
	apikey := d.Get("apikey").(string)

	options := ApplicationOptions{}
	options.name = d.Get("name").(string)
	rawRecipeTags := d.Get("recipe_tags").([]interface{})
	options.recipeTags = make([]string, len(rawRecipeTags))
	for i, raw := range rawRecipeTags {
		options.recipeTags[i] = raw.(string)
	}

	options.url = m.(ProviderConfig).Address
	options.apikey = m.(ProviderConfig).APIKey
	if address != "" {
		options.url = address
	}
	if apikey != "" {
		options.apikey = apikey
	}

	options.deployment = d.Get("deployment").(string)
	options.application = d.Get("application").(string)
	options.namespace = d.Get("namespace").(string)
	options.deploymentToken = d.Get("deployment_token").(string)
	options.deploymentAddress = d.Get("deployment_address").(string)
	if options.deploymentAddress == "" {
		options.deploymentAddress = options.url
	}
	cloudinit, err := createApp(options)
	if err != nil {
		return err
	}

	id := fmt.Sprintf("%s-%s", options.deployment, options.application)
	if options.name != "" {
		id = fmt.Sprintf("%s-%s", id, options.name)
	}
	d.SetId(id)
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
	// scripts := make([]string, 0)
	scripts := make([]terraModel.Recipe, 0)
	// Loop over app recipes, and go over parent recipes
	jr, _ := json.Marshal(respAppInfo)
	log.Printf("[INFO] app = %s", jr)

	scriptTxt := goterraTmplPre + "\n"

	// ISSUE varValue is label not value given at runtime
	// TODO Should match against a set of input map taken from vars, Inputs value is only a descriptive here
	// Or get them via env,?
	// get from run,  but run id is unknown (from os? added var?)
	// get from an optional input var
	// If needs var, template should have some goterra_push and recipe fetch it with goterra cli

	/*
		for varName, varValue := range respAppInfo.App.Inputs {
			if strings.HasPrefix(varName, "env_") {
				exportName := strings.Replace(varName, "env_", "", 1)
				scriptTxt += fmt.Sprintf("export %s=%q\n", exportName, varValue)
			}
		}
	*/
	if _, err := os.Stat("goterra.env"); err == nil {
		dat, err := ioutil.ReadFile("goterra.env")
		if err == nil {
			var inputs map[string]string
			if errJSON := json.Unmarshal(dat, &inputs); errJSON == nil {
				for key, val := range inputs {
					scriptTxt += fmt.Sprintf("export %s=%q\n", key, val)
					if key == "ssh_pub_key" && val != "" {
						quotedVal := strconv.Quote(val)
						scriptTxt += fmt.Sprintf("echo %s >> ~/.ssh/authorized_keys\n", quotedVal)
					}
				}
			}
		}

	}

	gotName := respAppInfo.App.Name
	if options.name != "" {
		gotName = options.name
	}

	for _, appRecipe := range respAppInfo.App.Recipes {
		recipe, err := getRecipe(options, appRecipe)
		if options.recipeTags != nil && len(options.recipeTags) > 0 {
			tagMatch := false
			if len(recipe.Tags) == 0 {
				// If no tag, consider it is a match
				tagMatch = true
			} else {
				log.Printf("[ERROR] OSALLOU SEARCH FOR RECIPES")
				for _, tag := range recipe.Tags {
					for _, appTag := range options.recipeTags {
						if tag == appTag {
							tagMatch = true
							break
						}
					}
					if tagMatch {
						break
					}
				}
			}
			if !tagMatch {
				continue
			}
		}
		if err != nil {
			log.Printf("[ERROR] Failed to get recipe")
			return "", fmt.Errorf("[ERROR] failed to get recipe")
		}
		scriptTxt += fmt.Sprintf("\n#*** Apply recipe %s **********\n", recipe.Name)

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
					scripts = append(scripts, parentRecipe)
					scriptTxt += fmt.Sprintf("\n#*** Load parent recipe %s:%s **********\n", parentRecipe.Name, parentRecipe.ID.Hex())
				}
			}
		}

		if _, ok := loadedScripts[recipe.Name]; ok {
			// Already loaded
		} else {
			loadedScripts[recipe.Name] = true
			scripts = append(scripts, *recipe)
			scriptTxt += fmt.Sprintf("\n#*** Load recipe %s:%s **********\n", recipe.Name, recipe.ID.Hex())
		}

		// for i := len(scripts) - 1; i >= 0; i-- {
		for i := 0; i < len(scripts); i++ {
			scripts[i].Script = strings.Replace(scripts[i].Script, "${GOT_ID}", options.application, -1)
			scripts[i].Script = strings.Replace(scripts[i].Script, "${GOT_URL}", options.deploymentAddress, -1)
			scripts[i].Script = strings.Replace(scripts[i].Script, "${GOT_TOKEN}", options.deploymentToken, -1)
			scripts[i].Script = strings.Replace(scripts[i].Script, "${GOT_DEP}", options.deployment, -1)
			scripts[i].Script = strings.Replace(scripts[i].Script, "${GOT_NAME}", gotName, -1)

			errRecipe := addRecipe(options, scripts[i].ID.Hex(), scripts[i].Script)
			if errRecipe != nil {
				return "", errRecipe
			}
			recipeIndex := "_recipe" + fmt.Sprintf("%s_%s", options.application, scripts[i].ID.Hex())
			scriptTxt += "\n" + "/opt/got/goterra-cli --deployment ${GOT_DEP} --url ${GOT_URL} --token $TOKEN get " + recipeIndex + " > /opt/got/" + recipeIndex + ".sh\n"
			scriptTxt += "dos2unix /opt/got/" + recipeIndex + ".sh\n"
			scriptTxt += "chmod +x /opt/got/" + recipeIndex + ".sh\n"
			scriptTxt += "/opt/got/" + recipeIndex + ".sh &>> /opt/got/${GOT_ID}.log"
		}

		scriptTxt += "\n#****************************\n"

		scripts = make([]terraModel.Recipe, 0)

	}

	scriptTxt = scriptTxt + "\n" + goterraTmpPost
	// write cloudinit file
	cloudinit = options.application + ".sh"
	if options.name != "" {
		cloudinit = fmt.Sprintf("%s-%s.sh", options.application, options.name)
	}

	scriptTxt = strings.Replace(scriptTxt, "${GOT_ID}", options.application, -1)
	scriptTxt = strings.Replace(scriptTxt, "${GOT_URL}", options.deploymentAddress, -1)
	scriptTxt = strings.Replace(scriptTxt, "${GOT_TOKEN}", options.deploymentToken, -1)
	scriptTxt = strings.Replace(scriptTxt, "${GOT_DEP}", options.deployment, -1)
	scriptTxt = strings.Replace(scriptTxt, "${GOT_NAME}", gotName, -1)

	errFile := ioutil.WriteFile(cloudinit, []byte(scriptTxt), 0644)
	if errFile != nil {
		return "", fmt.Errorf("[ERROR] failed to write cloudinit file")
	}
	return cloudinit, nil
}

func addRecipe(options ApplicationOptions, recipe string, script string) error {
	remote := []string{options.deploymentAddress, "store", options.deployment}
	recipeData := DeploymentData{Key: "_recipe" + fmt.Sprintf("%s_%s", options.application, recipe), Value: script}
	byteData, _ := json.Marshal(recipeData)
	req, _ := http.NewRequest("PUT", strings.Join(remote, "/"), bytes.NewBuffer(byteData))
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", options.deploymentToken))
	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("failed to contact server %s\n", options.url)
		return fmt.Errorf("[ERROR] Failed to store recipe %s: %s", recipe, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		log.Printf("failed to create deployment %d\n", resp.StatusCode)
		return fmt.Errorf("[ERROR] Failed to store recipe %s: %d", recipe, resp.StatusCode)
	}
	return nil
}

func getParentRecipe(options ApplicationOptions, recipeID string) (recipes []terraModel.Recipe, err error) {
	log.Printf("[INFO] load parent recipes of %s", recipeID)
	recipes = make([]terraModel.Recipe, 0)
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

func getRecipe(options ApplicationOptions, recipeID string) (recipe *terraModel.Recipe, err error) {
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
	Recipe terraModel.Recipe `json:"recipe"`
}

type RespApplication struct {
	App terraModel.Application `json:"app"`
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
	name              string
	recipeTags        []string
}
