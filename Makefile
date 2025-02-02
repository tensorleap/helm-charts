.PHONY: create-cluster drop-cluster helm-install helm-uninstall helm-reinstall helm-deps-up validate-k-env

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
