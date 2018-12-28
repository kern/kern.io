workflow "New workflow" {
  on = "push"
  resolves = ["GitHub Action for Google Cloud SDK auth"]
}

action "GitHub Action for Google Cloud SDK auth" {
  uses = "actions/gcloud/auth@8ec8bfa"
  runs = "make push"
}
