workflow "Build and push" {
  on = "push"
  resolves = ["GitHub Action for Google Cloud"]
}

action "Load credentials" {
  uses = "docker://github/gcloud"
  args = "container clusters get-credentials example-project --zone us-central1-a --project data-services-engineering"
}

action "GitHub Action for Google Cloud" {
  uses = "actions/gcloud/cli@8ec8bfa"
  secrets = ["GCLOUD_AUTH"]
  args = "cp -R 'build/*' gs://kern.io"
  runs = "gsutil"
}
