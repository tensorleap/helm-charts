.PHONY: create-cluster drop-cluster helm-install helm-uninstall helm-reinstall helm-deps-up validate-k-env

CLUSTER_NAME ?= tensorleap

cluster-create:
validate-k-env:
	[[ -x "$$(command -v kubectx)" && "$$(kubectx --current)" == 'k3d-tensorleap' ]]
	[[ -x "$$(command -v kubens)" && "$$(kubens --current)" == 'tensorleap' ]]

cluster-del:
	k3d cluster create ${CLUSTER_NAME} --k3s-arg="--disable=traefik@server:0"

helm-install: ./charts/tensorleap
	k3d cluster delete ${CLUSTER_NAME}

helm-uninstall:
	helm upgrade --install ${CLUSTER_NAME} ./charts/tensorleap

	helm uninstall ${CLUSTER_NAME}
helm-reinstall: helm-uninstall helm-install

helm-deps-up: ./charts/tensorleap
	helm dependency update ./charts/tensorleap
