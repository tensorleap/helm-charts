.PHONY: create-cluster drop-cluster helm-install helm-uninstall helm-reinstall helm-deps-up validate-k-env

CLUSTER_NAME ?= tensorleap

validate-k-env:
	[[ -x "$$(command -v kubectx)" && "$$(kubectx --current)" == 'k3d-tensorleap' ]]
	[[ -x "$$(command -v kubens)" && "$$(kubens --current)" == 'tensorleap' ]]

cluster-create:
	k3d cluster create ${CLUSTER_NAME} --config ./config/k3d-config.yaml

cluster-del: validate-k-env
	k3d cluster delete ${CLUSTER_NAME}

helm-install: validate-k-env
	helm upgrade --install ${CLUSTER_NAME} ./charts/tensorleap

helm-uninstall: validate-k-env
	helm uninstall ${CLUSTER_NAME}

helm-reinstall: helm-uninstall helm-install

helm-deps-up: validate-k-env
	helm dependency update ./charts/tensorleap
