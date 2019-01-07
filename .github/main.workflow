workflow "Build and push" {
  on = "push"
  resolves = [
    "Deploy",
    "GitHub Action for Google Cloud",
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

action "Deploy" {
  uses = "docker://google/cloud-sdk:latest"
  secrets = ["GCLOUD_AUTH"]
  needs = ["Build"]
  args = "echo \"$GCLOUD_AUTH\" > ~/.gcloud-auth.json && gsutil cp -R build/* gs://kern.io && ls"
  runs = "/bin/bash -c"
}

action "GitHub Action for Google Cloud" {
  uses = "actions/gcloud/cli@8ec8bfa"
  needs = ["Build"]
  args = "ls && ls"
  secrets = ["GCLOUD_AUTH"]
}
