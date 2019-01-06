workflow "Build and push" {
  on = "push"
  resolves = ["Deploy"]
}

action "Load credentials" {
  uses = "docker://github/gcloud"
  args = "container clusters get-credentials example-project --zone us-central1-a --project data-services-engineering"
}

action "Install" {
  uses = "docker://node:latest"
  args = "npm install"
}

action "Build" {
  uses = "docker://node:latest"
  args = "./node_modules/.bin/webpack -p --output-path=build"
  needs = ["Install"]
}

action "Deploy" {
  uses = "actions/gcloud/cli@8ec8bfa"
  args = "\"gsutil cp -R 'build/*' gs://kern.io\""
  secrets = ["GCLOUD_AUTH"]
  needs = ["Build"]
  runs = "sh -c"
}
