set -euo pipefail


INSTALL_ID=$RANDOM$RANDOM
UNAME=$(uname -a)
ARCHITECTURE=$(uname -m)

report_status() {
  curl -s -XPOST https://us-central1-tensorleap-ops3.cloudfunctions.net/demo-contact-bot -H 'Content-Type: application/json' -d "$1" &> /dev/null &
}
report_status "{\"type\":\"install-script-init\",\"installId\":\"$INSTALL_ID\",\"uname\":\"$UNAME\"}"

if [ "$ARCHITECTURE" == "arm64" ];
then
  report_status "{\"type\":\"install-script-apple-silicon\",\"installId\":\"$INSTALL_ID\"}"
  echo 'Apple M1 support will be available soon. Drop us a line at \033[1minfo@tensorleap.ai\033[0m and we will notify you as soon as it is ready.'
  exit -1
fi

# Install k3d
echo Checking k3d installation
if !(k3d version);
then
  echo Installing k3d...
  report_status "{\"type\":\"install-script-install-k3d\",\"installId\":\"$INSTALL_ID\"}"
  curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
fi

if !(docker container list &> /dev/null);
then
  report_status "{\"type\":\"install-script-docker-not-running\",\"installId\":\"$INSTALL_ID\"}"
  echo Docker is not running!
  echo Please install and run docker, get it at $(tput bold)https://docs.docker.com/get-docker/
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
    report_status "{\"type\":\"install-script-up-to-date\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
    echo Installation in up to date!
    exit 0
  fi

  report_status "{\"type\":\"install-script-update-started\",\"installId\":\"$INSTALL_ID\",\"from\":\"$INSTALLED_CHART_VERSION\",\"to\":\"$LATEST_CHART_VERSION\"}"
  echo Installed Version: $INSTALLED_CHART_VERSION
  echo Updating to latest version...
  docker exec -it k3d-tensorleap-server-0 kubectl patch -n kube-system  HelmChart/tensorleap --type='merge' -p "{\"spec\":{\"version\":\"$LATEST_CHART_VERSION\"}}"
  report_status "{\"type\":\"install-script-update-success\",\"installId\":\"$INSTALL_ID\",\"from\":\"$INSTALLED_CHART_VERSION\",\"to\":\"$LATEST_CHART_VERSION\"}"

  # Download engine latest image
  LATEST_ENGINE_IMAGE=$(curl -s https://raw.githubusercontent.com/tensorleap/helm-charts/master/engine-latest-image)
  docker exec -it k3d-tensorleap-server-0 kubectl create job -n tensorleap engine-download-$INSTALL_ID --image=$LATEST_ENGINE_IMAGE -- sh -c "echo Downloaded $LATEST_ENGINE_IMAGE" &> /dev/null

  echo 'Done! (note that images could still be downloading in the background...)'
else

  echo Checking docker storage and memory limits...

  REQUIRED_MEMORY=6227000000
  REQUIRED_MEMORY_PRETTY=6Gb
  DOCKER_MEMORY=$(docker info -f '{{json .MemTotal}}')
  DOCKER_MEMORY_PRETTY="$(echo "scale=2; $DOCKER_MEMORY /1024/1024/1024" | bc -l)Gb"

  REQUIRED_STORAGE_KB=41943040
  REQUIRED_STORAGE_PRETTY=40Gb
  docker pull -q alpine &> /dev/null
  DF_OUTPUT=$(docker run --rm -it alpine df -t overlay -P | grep overlay | sed 's/  */:/g')
  DOCKER_TOTAL_STORAGE_KB=$(echo $DF_OUTPUT | cut -f2 -d:)
  DOCKER_TOTAL_STORAGE_PRETTY="$(echo "scale=2; $DOCKER_TOTAL_STORAGE_KB /1024/1024" | bc -l)Gb"
  DOCKER_FREE_STORAGE_KB=$(echo $DF_OUTPUT | cut -f4 -d:)
  DOCKER_FREE_STORAGE_PRETTY="$(echo "scale=2; $DOCKER_FREE_STORAGE_KB /1024/1024" | bc -l)Gb"

  NO_RESOURCES=''
  if [ $DOCKER_MEMORY -lt $REQUIRED_MEMORY ];
  then
    echo "Please increase docker memory limit to $REQUIRED_MEMORY_PRETTY (Current limit is $DOCKER_MEMORY_PRETTY)"
    NO_RESOURCES=true
  fi

  if [ $DOCKER_FREE_STORAGE_KB -lt $REQUIRED_STORAGE_KB ];
  then
    echo "Please increase docker storage limit, tensorleap required at least $REQUIRED_STORAGE_PRETTY free storage (Currently $DOCKER_FREE_STORAGE_PRETTY is available, total $DOCKER_TOTAL_STORAGE_PRETTY)"
    NO_RESOURCES=true
  fi

  if [ -n "$NO_RESOURCES" ];
  then
    report_status "{\"type\":\"install-script-no-resources\",\"installId\":\"$INSTALL_ID\",\"totalMemory\":\"$DOCKER_MEMORY_PRETTY\",\"totalStorage\":\"$DOCKER_TOTAL_STORAGE_PRETTY\",\"freeStorage\":\"$DOCKER_FREE_STORAGE_PRETTY\"}"
    echo Please retry installation after updating your docker config.
    exit -1
  fi

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

  VOLUME_ENGINE_VALUES=""
  if [ -n "$VOLUME" ]
  then
    VOLUME_ENGINE_VALUES="localDataDirectory: ${VOLUME/*:/}"
  fi

  VOLUMES_MOUNT_PARAM=$([ -z $VOLUME ] && echo '' || echo "-v $VOLUME")

  USE_GPU=${USE_GPU:=}
  GPU_CLUSTER_PARAMS=""
  GPU_ENGINE_VALUES=""
  if [ "$USE_GPU" == "true" ]
  then
    GPU_CLUSTER_PARAMS='--image gcr.io/tensorleap/k3s:v1.23.8-k3s1-cuda --gpus all'
    GPU_ENGINE_VALUES='gpu: true'
  fi

  VALUES_CONTENT=""
  if [ -n "$VOLUME_ENGINE_VALUES$GPU_ENGINE_VALUES" ]
  then
    VALUES_CONTENT=$(cat << EOF
  valuesContent: |-
    tensorleap-engine:
      ${VOLUME_ENGINE_VALUES}
      ${GPU_ENGINE_VALUES}
EOF
)
  fi


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
  report_status "{\"type\":\"install-script-creating-cluster\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\",\"volume\":\"$VOLUME\"}"
  k3d cluster create tensorleap \
    --k3s-arg='--disable=traefik@server:0' $GPU_CLUSTER_PARAMS \
    -p "$PORT:80@loadbalancer" \
    -v $HOME/.config/tensorleap/manifests/tensorleap.yaml:/var/lib/rancher/k3s/server/manifests/tensorleap.yaml $VOLUMES_MOUNT_PARAM

  # Download engine latest image
  LATEST_ENGINE_IMAGE=$(curl -s https://raw.githubusercontent.com/tensorleap/helm-charts/master/engine-latest-image)
  docker exec -it k3d-tensorleap-server-0 kubectl create job -n tensorleap engine-download-$INSTALL_ID --image=$LATEST_ENGINE_IMAGE -- sh -c "echo Downloaded $LATEST_ENGINE_IMAGE" &> /dev/null

  echo 'Waiting for images to download and install... (This can take up to 15 minutes depends on network speed)'
  report_status "{\"type\":\"install-script-helm-install-wait\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  if !(docker exec -it k3d-tensorleap-server-0 kubectl wait --for=condition=complete --timeout=25m -n kube-system job helm-install-tensorleap);
  then
    report_status "{\"type\":\"install-script-helm-install-timeout\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
    echo "Timeout! Images may still be downloading, wait a few minutes and see if Tensorleap is available on http://127.0.0.1:$PORT If it's not, contact support"
    exit -1
  fi
  echo 'Waiting for containers to initialize... (Just a few more minutes!)'
  report_status "{\"type\":\"install-script-deployment-wait\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  if !(docker exec -it k3d-tensorleap-server-0 kubectl wait --for=condition=available --timeout=25m -n tensorleap deploy -l app.kubernetes.io/managed-by=Helm);
  then
    report_status "{\"type\":\"install-script-deployment-timeout\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
    echo "Timeout! Images may still be downloading, wait a few minutes and see if Tensorleap is available on http://127.0.0.1:$PORT If it's not, contact support"
    exit -1
  fi

  report_status "{\"type\":\"install-script-install-success\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  echo Tensorleap demo installed! It should be available now on http://127.0.0.1:$PORT
fi
