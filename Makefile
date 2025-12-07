.PHONY: create-cluster drop-cluster helm-install helm-uninstall helm-reinstall helm-deps-up validate-k-env release-notes

SHELL := /bin/bash
CLUSTER_NAME ?= tensorleap
NAME_SPACE ?= tensorleap

validate-k-env:
	[[ -x "$$(command -v kubectx)" && "$$(kubectx --current)" == 'k3d-tensorleap' ]]
	[[ -x "$$(command -v kubens)" && "$$(kubens --current)" == 'tensorleap' ]]

cluster-create:
	k3d cluster create ${CLUSTER_NAME} --config ./config/k3d-config.yaml

cluster-del: validate-k-env
	k3d cluster delete ${CLUSTER_NAME}

helm-install: validate-k-env
	helm upgrade --install ${CLUSTER_NAME} ./charts/tensorleap -n ${NAME_SPACE}

helm-uninstall: validate-k-env
	helm uninstall ${CLUSTER_NAME} -n ${NAME_SPACE}

helm-reinstall: helm-uninstall helm-install

helm-deps-up: validate-k-env
	helm dependency update ./charts/tensorleap -n ${NAME_SPACE}

.PHONY: lint
lint:
	@golangci-lint run

.PHONY: fmt
fmt:
	@gofmt -w -l ./

create_go_tag:
	git tag -a $$(go run . --version) -m"$$(go run . --version)"

.PHONY: check-fmt
check-fmt:
	@echo "Checking code formatting..."
	@result=$$(gofmt -l ./); \
	if [ -n "$$result" ]; then \
		echo "Formatting issues found:"; \
		echo "$$result"; \
		exit 1; \
	fi

.PHONY: test
test:
	@go test ./...

# This code run helm template on charts and extracts all image names by simple search of image: [image-name]
.PHONY: update-images
update-images:
	(helm template ./charts/tensorleap-infra --set nvidiaGpu.enabled=true && helm template ./charts/tensorleap) \
		| grep 'image: ' \
		| sed 's/.*: //' \
		| sed 's/\"//g' \
		| sort \
		| uniq > images.txt

.PHONY: build-helm
build-helm:
	helm repo add nginx https://kubernetes.github.io/ingress-nginx
	helm repo add elastic https://helm.elastic.co
	helm repo add minio https://charts.min.io
	helm repo add codecentric https://codecentric.github.io/helm-charts
	helm repo add datadog https://helm.datadoghq.com
	helm dependency build ./charts/tensorleap
	rm ./charts/tensorleap/Chart.lock
	helm dependency build ./charts/tensorleap-infra
	rm ./charts/tensorleap-infra/Chart.lock



.PHONY: checkout-rc-branch
.ONESHELL:
checkout-rc-branch:
	@set -euo pipefail
	if [ ! -f charts/tensorleap/Chart.yaml ]; then
	  echo "❌ charts/tensorleap/Chart.yaml not found" >&2
	  exit 1
	fi
	VERSION_FULL="$$(awk '/^version:/{print $$2}' charts/tensorleap/Chart.yaml)"
	if [ -z "$$VERSION_FULL" ]; then
	  echo "❌ version not found in charts/tensorleap/Chart.yaml" >&2
	  exit 1
	fi
	# Remove -rc.* suffix if present to get base version
	VERSION=$$(echo "$$VERSION_FULL" | sed 's/-rc\.[0-9]*$$//')
	git fetch origin master --prune >/dev/null 2>&1
	# Branch name is just the base version (e.g., 1.4.75)
	BRANCH="$$VERSION"
	CURRENT_BRANCH="$$(git rev-parse --abbrev-ref HEAD)"
	# Check if we're already on the version branch
	if [ "$$CURRENT_BRANCH" != "$$BRANCH" ]; then
	  # Checkout or create the version branch
	  if git ls-remote --exit-code --heads origin "$$BRANCH" >/dev/null 2>&1; then
	    git fetch origin "$$BRANCH" >/dev/null 2>&1
	    git switch "$$BRANCH" >/dev/null 2>&1
	  else
	    git switch -c "$$BRANCH" origin/master >/dev/null 2>&1
	    git push -u origin "$$BRANCH" >/dev/null 2>&1
	  fi
	fi
	# Find the next RC number by checking existing tags (fetch tags first)
	git fetch origin --tags >/dev/null 2>&1
	EXISTING_TAGS="$$(git tag -l "$${VERSION}-rc.*" 2>/dev/null | sed -nE "s/^$${VERSION}-rc\.([0-9]+)$$/\1/p")"
	if [ -z "$$EXISTING_TAGS" ]; then
	  NEXT=0
	else
	  MAX_RC="$$(printf "%s\n" "$$EXISTING_TAGS" | sort -n | tail -1)"
	  NEXT=$$((MAX_RC+1))
	fi
	# Update Chart.yaml version to include RC suffix (matches tag)
	VERSION_WITH_RC="$${VERSION}-rc.$${NEXT}"
	sed -i.bak "s/^version: .*/version: $$VERSION_WITH_RC/" charts/tensorleap/Chart.yaml
	rm -f charts/tensorleap/Chart.yaml.bak
	# Output only the branch name (for use in workflows)
	echo "$$BRANCH"

.PHONY: remove-rc-suffix
remove-rc-suffix:
	@set -euo pipefail
	# Remove rc.* suffix from tensorleap chart version if present (e.g., 1.2.3-rc.0 -> 1.2.3)
	CHART_FILE="charts/tensorleap/Chart.yaml"
	if [ ! -f "$$CHART_FILE" ]; then
	  echo "❌ $$CHART_FILE not found" >&2
	  exit 1
	fi
	CURRENT_VERSION="$$(grep -E '^version:' "$$CHART_FILE" | awk '{print $$2}')"
	if [[ "$$CURRENT_VERSION" =~ -rc\. ]]; then
	  CLEAN_VERSION="$$(echo "$$CURRENT_VERSION" | sed 's/-rc\.[0-9]*$$//')"
	  sed -i.bak "s/^version: .*/version: $$CLEAN_VERSION/" "$$CHART_FILE"
	  rm -f "$${CHART_FILE}.bak"
	  echo "Updated tensorleap chart version: $$CURRENT_VERSION -> $$CLEAN_VERSION"
	else
	  echo "Tensorleap chart version has no rc suffix: $$CURRENT_VERSION"
	fi

.PHONY: release-notes
release-notes:

	@python3 scripts/release-note-generator.py

