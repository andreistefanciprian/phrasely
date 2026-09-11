TF_TARGET ?= cloudflare
TF_PLAN_FILE ?= $(TF_TARGET).tfplan
TF := TF_TARGET=$(TF_TARGET) docker compose run --rm terraform

.PHONY: init fmt validate check plan apply output

init:
	$(TF) init

fmt:
	$(TF) fmt -check

validate:
	$(TF) validate

check: fmt validate

plan:
	$(TF) plan -out=$(TF_PLAN_FILE)

apply:
	$(TF) apply $(TF_PLAN_FILE)

output:
	$(TF) output
