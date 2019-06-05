resource "goterra_deployment" "my-deploy" {
    address = "http://localhost:8000"
    apikey = "5FBBD6P84OE7UT4QRTTE"
}

resource "goterra_push" "key1" {
  address = "${goterra_deployment.my-deploy.address}"
  token = "${goterra_deployment.my-deploy.token}"
  deployment = "${goterra_deployment.my-deploy.id}"
  key = "key1"
  value = "value1"

  depends_on = ["goterra_deployment.my-deploy"]
}

resource "goterra_application" "app1" {
  address = "http://localhost:8002"
  apikey = "${goterra_deployment.my-deploy.apikey}"
  deployment = "${goterra_deployment.my-deploy.id}"
  deployment_token = "${goterra_deployment.my-deploy.token}"
  deployment_address = "${goterra_deployment.my-deploy.address}"

  namespace = "5cf51c96f01f06317bdb3d51"
  application = "5cf78f2183a8fb8c505c524a"

  depends_on = ["goterra_deployment.my-deploy"]
}

data "goterra_deployment" "toto" {
    address = "${goterra_deployment.my-deploy.address}"
    deployment = "${goterra_deployment.my-deploy.id}"
    token = "${goterra_deployment.my-deploy.token}"
    key = "toto"

    depends_on = ["goterra_deployment.my-deploy"]
}

output "deployment_token" {
  value = "${goterra_deployment.my-deploy.token}"
  depends_on = ["goterra_deployment.my-deploy"]
}

output "deployment_id" {
  value = "${goterra_deployment.my-deploy.id}"
  depends_on = ["goterra_deployment.my-deploy"]

}

output "toto_output" {
  value = "${data.goterra_deployment.toto.data}"
  depends_on = ["data.goterra_deployment.toto"]
}
