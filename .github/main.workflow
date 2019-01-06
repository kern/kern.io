workflow "Build and push" {
  on = "push"
  resolves = ["GitHub Action for Google Cloud"]
}

action "Load credentials" {
  uses = "docker://github/gcloud"
  args = "container clusters get-credentials example-project --zone us-central1-a --project data-services-engineering"
}

action "GitHub Action for npm" {
  uses = "actions/npm@e7aaefe"
  args = "install"
}

action "GitHub Action for npm-1" {
  uses = "actions/npm@e7aaefe"
  needs = ["GitHub Action for npm"]
  args = "-p --output-path=build"
  runs = "webpack"
}

action "GitHub Action for Google Cloud" {
  uses = "actions/gcloud/cli@8ec8bfa"
  needs = ["GitHub Action for npm-1"]
  runs = "gsutil"
  args = "cp -R 'build/*' gs://kern.io"
  secrets = ["GCLOUD_AUTH"]
}
