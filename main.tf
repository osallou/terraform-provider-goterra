provider "goterra" {
  address = "http://localhost:8000"
  apikey = "5FBBD6P84OE7UT4QRTTE"
}

resource "goterra_deployment" "my-deploy" {

}


output "deployment_token" {
  value = "${goterra_deployment.my-deploy.token}"
  depends_on = ["goterra_deployment.my-deploy"]
}

output "deployment_id" {
  value = "${goterra_deployment.my-deploy.id}"
  depends_on = ["goterra_deployment.my-deploy"]

}

