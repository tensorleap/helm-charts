.PHONY: create-cluster drop-cluster helm-install helm-uninstall helm-reinstall helm-deps-up

NAMESPACE ?= tensorleap

cluster-create:
	k3d cluster create ${NAMESPACE} --k3s-arg="--disable=traefik@server:0"

cluster-del:
	k3d cluster delete ${NAMESPACE}

helm-install: ./charts/tensorleap
	helm upgrade --install ${NAMESPACE} ./charts/tensorleap

helm-uninstall:
	helm uninstall ${NAMESPACE}

helm-reinstall: helm-uninstall helm-install

helm-deps-up: ./charts/tensorleap
	helm dependency update ./charts/tensorleap
