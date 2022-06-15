set -euo pipefail

# Install k3d
echo Checking k3d installation
if !(k3d version);
then
  echo Installing k3d...
  curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
fi

if !(docker container list &> /dev/null);
then
  echo Docker is not running!
  exit -1
fi

echo Getting latest version...
LATEST_CHART_VERSION=$(curl -s https://raw.githubusercontent.com/tensorleap/helm-charts/master/charts/tensorleap/Chart.yaml | grep '^version:' | cut -c 10-)
echo $LATEST_CHART_VERSION

if k3d cluster list tensorleap &> /dev/null;
then
  echo Detected existing tensorleap installation
  INSTALLED_CHART_VERSION=$(docker exec -it k3d-tensorleap-server-0 kubectl get -n kube-system HelmChart tensorleap -o jsonpath='{.spec.version}')
  if [ "$LATEST_CHART_VERSION" == "$INSTALLED_CHART_VERSION" ]
  then
    echo Installation in up to date!
    exit 0
  fi

  echo Installed Version: $INSTALLED_CHART_VERSION
  echo Updating to latest version...
  docker exec -it k3d-tensorleap-server-0 kubectl patch -n kube-system  HelmChart/tensorleap --type='merge' -p "{\"spec\":{\"version\":\"$LATEST_CHART_VERSION\"}}"
  echo 'Done! (note that images could still be downloading in the background...)'
else
  # Get port and volume mount
  PORT=${TENSORLEAP_PORT:=4589}
  VOLUME=${TENSORLEAP_VOLUME:=}
  if [ -z "$VOLUME" ]
  then
    echo "Enter a path to be mounted and accessible by scripts (leave empty to skip):"
    read LOCAL_PATH
    if [ -n "$LOCAL_PATH" ]
    then
      echo "Enter the path on the container: (leave empty to use same path):"
      read CONTAINER_PATH
      LOCAL_PATH=$(cd $LOCAL_PATH && pwd)
      VOLUME="$LOCAL_PATH:${CONTAINER_PATH:=$LOCAL_PATH}"
    fi
  fi

  VALUES_CONTENT=$([ -z $VOLUME ] && echo '' || cat << EOF
  valuesContent: |-
    tensorleap-engine:
      localDataDirectory: ${VOLUME/*:/}
EOF
)

  VOLUMES_MOUNT_PARAM=$([ -z $VOLUME ] && echo '' || echo "-v $VOLUME")
  mkdir -p $HOME/.config/tensorleap/manifests

  cat << EOF > $HOME/.config/tensorleap/manifests/tensorleap.yaml
apiVersion: helm.cattle.io/v1
kind: HelmChart
metadata:
  name: tensorleap
  namespace: kube-system
spec:
  chart: tensorleap
  repo: https://helm.tensorleap.ai
  version: $LATEST_CHART_VERSION
  targetNamespace: tensorleap
$VALUES_CONTENT
---
apiVersion: v1
kind: Namespace
metadata:
  name: tensorleap
EOF

  echo Creating tensorleap k3d cluster...
  k3d cluster create tensorleap \
    --k3s-arg='--disable=traefik@server:0' \
    -p "$PORT:80@loadbalancer" \
    -v $HOME/.config/tensorleap/manifests/tensorleap.yaml:/var/lib/rancher/k3s/server/manifests/tensorleap.yaml $VOLUMES_MOUNT_PARAM

  echo 'Waiting for images to download and install... (This can take up to 15 minutes depends on network speed)'
  if !(docker exec -it k3d-tensorleap-server-0 kubectl wait --for=condition=complete --timeout=25m -n kube-system job helm-install-tensorleap);
  then
    echo "Timeout! Images may still be downloading, wait a few minutes and see if Tensorleap is available on http://127.0.0.1:$PORT If it's not, contact support"
    exit -1
  fi
  echo 'Waiting for containers to initialize... (Just a few more minutes!)'
  if !(docker exec -it k3d-tensorleap-server-0 kubectl wait --for=condition=available --timeout=25m -n tensorleap deploy -l app.kubernetes.io/managed-by=Helm);
  then
    echo "Timeout! Images may still be downloading, wait a few minutes and see if Tensorleap is available on http://127.0.0.1:$PORT If it's not, contact support"
    exit -1
  fi

  echo Tensorleap demo installed! It should be available now on http://127.0.0.1:$PORT
fi
