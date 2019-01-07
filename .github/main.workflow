workflow "Build and push" {
  on = "push"
  resolves = [
    "GitHub Action for Google Cloud",
    "Build",
  ]
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

action "GitHub Action for Google Cloud" {
  uses = "actions/gcloud/auth@8ec8bfa"
  needs = ["Build"]
  args = "&& gsutil cp -R build/* gs://kern.io"
  secrets = ["GCLOUD_AUTH"]
}
