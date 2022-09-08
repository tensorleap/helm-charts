set -euo pipefail

DISABLE_REPORTING=${DISABLE_REPORTING:=}

INSTALL_ID=$RANDOM$RANDOM
DOCKER=docker
K3D=k3d

VAR_DIR='/var/lib/tensorleap/standalone'
K3S_VAR_DIR='/var/lib/rancher/k3s'

REGISTRY_PORT=${TENSORLEAP_REGISTRY_PORT:=5699}

function setup_http_utils() {
  if type curl > /dev/null; then
    echo using curl
    HTTP_GET='curl -s --fail'
  elif type wget > /dev/null; then
    echo using wget
    HTTP_GET='wget -q -O-'
  else
    echo you must have either curl or wget installed.
    exit -1
  fi
}

function download_file() {
  if type curl > /dev/null; then
    curl -s --fail $1 -o $2
  elif type wget > /dev/null; then
    wget -q -O $2 $1
  else
    echo you must have either curl or wget installed.
    exit -1
  fi
}

function report_status() {
  local report_url=https://us-central1-tensorleap-ops3.cloudfunctions.net/demo-contact-bot
  if [ "$DISABLE_REPORTING" != "true" ]
  then
    if type curl > /dev/null; then
      curl -s --fail -XPOST -H 'Content-Type: application/json' $report_url -d "$1" &> /dev/null &
    elif type wget > /dev/null; then
      wget -q --method POST --header 'Content-Type: application/json' -O- --body-data "$1" $report_url &> /dev/null &
    else
      echo you must have either curl or wget installed.
      exit -1
    fi
  fi
}

function check_k3d() {
  echo Checking k3d installation
  if !(k3d version);
  then
    echo Installing k3d...
    report_status "{\"type\":\"install-script-install-k3d\",\"installId\":\"$INSTALL_ID\"}"
    $HTTP_GET https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
  fi
}

function check_docker() {
  OS_NAME=$(uname -s)
  echo Checking docker installation
  if !(which docker &> /dev/null);
  then
    if [ "$OS_NAME" == "Linux" ];
    then
      report_status "{\"type\":\"install-script-installing-docker\",\"installId\":\"$INSTALL_ID\",\"os\":\"$OS_NAME\"}"
      echo Running docker community installation script...
      $HTTP_GET https://get.docker.com | sh \
        && sleep 2
    elif [ "$OS_NAME" == "Darwin" ];
    then
      report_status "{\"type\":\"install-script-installing-docker\",\"installId\":\"$INSTALL_ID\",\"os\":\"$OS_NAME\"}"
      TEMP_DIR=$(mktemp -d)
      echo Downloading docker...
      $HTTP_GET https://desktop.docker.com/mac/main/amd64/Docker.dmg > $TEMP_DIR/Docker.dmg
      echo Installing docker...
      sudo hdiutil attach $TEMP_DIR/Docker.dmg \
        && sudo /Volumes/Docker/Docker.app/Contents/MacOS/install \
        && sudo hdiutil detach /Volumes/Docker \
        && sleep 2 \
        && open -a Docker

      echo "Waiting for docker to start... (You may be asked to allow privileged access to docker)"
      until docker ps &> /dev/null; do sleep 2; done

    else
      report_status "{\"type\":\"install-script-docker-not-installed\",\"installId\":\"$INSTALL_ID\",\"os\":\"$OS_NAME\"}"
      echo Please install and run docker, get it at $(tput bold)https://docs.docker.com/get-docker/
      exit -1
    fi
  fi

  if !(docker ps &> /dev/null);
  then
    if !(sudo docker ps &> /dev/null);
    then
      report_status "{\"type\":\"install-script-docker-not-running\",\"installId\":\"$INSTALL_ID\",\"os\":\"$OS_NAME\"}"
      echo 'Docker is not running!'
      exit -1
    fi

    DOCKER='sudo docker'
    K3D='sudo k3d'
  fi
}

function get_latest_chart_version() {
  echo Getting latest version...
  LATEST_CHART_VERSION=$($HTTP_GET https://raw.githubusercontent.com/tensorleap/helm-charts/master/charts/tensorleap/Chart.yaml | grep '^version:' | cut -c 10-)
  echo $LATEST_CHART_VERSION
}

function run_in_docker() {
  $DOCKER exec -it k3d-tensorleap-server-0 $*
}

function create_docker_backups_folder() {
  run_in_docker mkdir -m 777 /mongodb-backups &> /dev/null || run_in_docker chmod -R 777 /mongodb-backups
}

function check_docker_requirements() {
  echo Checking docker storage and memory limits...

  REQUIRED_MEMORY=6227000000
  REQUIRED_MEMORY_PRETTY=6Gb
  DOCKER_MEMORY=$($DOCKER info -f '{{json .MemTotal}}')
  DOCKER_MEMORY_PRETTY="$(echo "scale=2; $DOCKER_MEMORY /1024/1024/1024" | bc -l)Gb"

  REQUIRED_STORAGE_KB=83886080
  REQUIRED_STORAGE_PRETTY=80Gb
  $DOCKER pull -q alpine &> /dev/null
  DF_TMP_FILE=$(mktemp)
  $DOCKER run --rm -it alpine df -t overlay -P > $DF_TMP_FILE
  DF_OUTPUT=$(cat $DF_TMP_FILE | grep overlay | sed 's/  */:/g')
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
}

function create_docker_registry() {
  if $K3D registry list tensorleap-registry &> /dev/null;
  then
    report_status "{\"type\":\"install-script-registry-exists\",\"installId\":\"$INSTALL_ID\"}"
    echo Found existing docker registry!
  else
    report_status "{\"type\":\"install-script-creating-registry\",\"installId\":\"$INSTALL_ID\"}"
    check_docker_requirements
    echo Creating docker registry...
    $K3D registry create tensorleap-registry -p $REGISTRY_PORT
  fi
}

function cache_image() {
  local registry_port=$1
  local image=$2
  local target=$(echo $image | sed "s/[^\/]*\//127.0.0.1:$registry_port\//" | sed 's/@.*$//')
  local api_url=$(echo $target | sed 's/\//\/v2\//' | sed 's/:/\/manifests\//2')
  if $HTTP_GET $api_url &> /dev/null;
  then
    echo "$image already cached"
  else
    $DOCKER pull $image && \
    $DOCKER tag $image $target && \
    $DOCKER push $target && \
    $DOCKER image rm $image
  fi
}
export HTTP_GET
export DOCKER
export -f cache_image

function cache_images_in_registry() {
  $HTTP_GET https://raw.githubusercontent.com/tensorleap/helm-charts/master/images.txt | xargs -P3 -IXXX bash -c "cache_image $REGISTRY_PORT XXX"
}

function get_installation_options() {
  # Get port and volume mount
  PORT=${TENSORLEAP_PORT:=4589}
  VOLUME=${TENSORLEAP_VOLUME:=}
  DEFAULT_VOLUME="$HOME/tensorleap/data"
  if [ -z "$VOLUME" ]
  then
    echo "Enter a path to be mounted and accessible by scripts (default: $DEFAULT_VOLUME):"
    read LOCAL_PATH
    if [ -n "$LOCAL_PATH" ]
    then
      echo "Enter the path on the container: (leave empty to use same path):"
      read CONTAINER_PATH
      LOCAL_PATH=$(cd $LOCAL_PATH && pwd)
      VOLUME="$LOCAL_PATH:${CONTAINER_PATH:=$LOCAL_PATH}"
    else
      mkdir -p $DEFAULT_VOLUME
      VOLUME="$DEFAULT_VOLUME:$DEFAULT_VOLUME"
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
}

function init_var_dir() {
  sudo mkdir -p $VAR_DIR
  sudo chmod -R 777 $VAR_DIR
  mkdir -p $VAR_DIR/manifests
  mkdir -p $VAR_DIR/storage
  mkdir -p $VAR_DIR/scripts

  echo 'Downloading config files...'
  download_file https://raw.githubusercontent.com/tensorleap/helm-charts/master/config/k3d-config.yaml $VAR_DIR/manifests/k3d-config.yaml
  download_file https://raw.githubusercontent.com/tensorleap/helm-charts/master/config/k3d-entrypoint.sh $VAR_DIR/scripts/k3d-entrypoint.sh
  chmod +x $VAR_DIR/scripts/k3d-entrypoint.sh
}

function create_tensorleap_helm_manifest() {
  cat << EOF > $VAR_DIR/manifests/tensorleap.yaml
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
}

function download_latest_engine_image() {
  local latest_engine_image=$($HTTP_GET https://raw.githubusercontent.com/tensorleap/helm-charts/master/engine-latest-image)
  local target=$(echo $latest_engine_image | sed "s/[^\/]*\//127.0.0.1:$REGISTRY_PORT\//" | sed 's/@.*$//')
  local api_url=$(echo $target | sed 's/\//\/v2\//' | sed 's/:/\/manifests\//2')
  if ! $HTTP_GET $api_url &> /dev/null;
  then
    $DOCKER run --rm -d --name tensorleap-engine-image-download-$INSTALL_ID \
      -v /var/run/docker.sock:/var/run/docker.sock \
      -e SOURCE=$latest_engine_image -e TARGET=$target \
      docker:cli \
      sh -c 'docker pull $SOURCE && docker tag $SOURCE $TARGET && docker push $TARGET' &> /dev/null
  fi
}

function create_tensorleap_cluster() {
  echo Creating tensorleap k3d cluster...
  report_status "{\"type\":\"install-script-creating-cluster\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\",\"volume\":\"$VOLUME\"}"
  $K3D cluster create --config $VAR_DIR/manifests/k3d-config.yaml \
    -p "$PORT:80@loadbalancer" $GPU_CLUSTER_PARAMS $VOLUMES_MOUNT_PARAM
}

function wait_for_cluster_init() {
  echo 'Setting up cluster... (this may take a few minutes)'
  report_status "{\"type\":\"install-script-helm-install-wait\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  if !(run_in_docker kubectl wait --for=condition=complete --timeout=25m -n kube-system job helm-install-tensorleap);
  then
    report_status "{\"type\":\"install-script-helm-install-timeout\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
    echo "Timeout! Cluster is starting in the background, wait a few minutes and see if Tensorleap is available on http://127.0.0.1:$PORT If it's not, contact support"
    exit -1
  fi
  echo 'Waiting for containers to initialize... (Just a few more minutes!)'
  report_status "{\"type\":\"install-script-deployment-wait\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  if !(run_in_docker kubectl wait --for=condition=available --timeout=25m -n tensorleap deploy -l app.kubernetes.io/managed-by=Helm);
  then
    report_status "{\"type\":\"install-script-deployment-timeout\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
    echo "Timeout! Cluster is starting in the background, wait a few minutes and see if Tensorleap is available on http://127.0.0.1:$PORT If it's not, contact support"
    exit -1
  fi
}

function check_installed_version() {
  INSTALLED_CHART_VERSION=$(run_in_docker kubectl get -n kube-system HelmChart tensorleap -o jsonpath='{.spec.version}')
  if [ "$LATEST_CHART_VERSION" == "$INSTALLED_CHART_VERSION" ]
  then
    report_status "{\"type\":\"install-script-up-to-date\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
    echo Installation in up to date!
    exit 0
  fi
  echo Installed Version: $INSTALLED_CHART_VERSION
}

function install_new_tensorleap_cluster() {
  get_installation_options
  create_docker_registry
  download_latest_engine_image
  cache_images_in_registry
  init_var_dir
  create_tensorleap_helm_manifest
  create_tensorleap_cluster
  create_docker_backups_folder
  wait_for_cluster_init

  report_status "{\"type\":\"install-script-install-success\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  echo "Tensorleap demo installed! It should be available now on http://127.0.0.1:$PORT"
}

function update_existing_chart() {
  cache_images_in_registry
  check_installed_version

  report_status "{\"type\":\"install-script-update-started\",\"installId\":\"$INSTALL_ID\",\"from\":\"$INSTALLED_CHART_VERSION\",\"to\":\"$LATEST_CHART_VERSION\"}"

  echo Updating to latest version...
  run_in_docker kubectl patch -n kube-system  HelmChart/tensorleap --type='merge' -p "{\"spec\":{\"version\":\"$LATEST_CHART_VERSION\"}}"
  report_status "{\"type\":\"install-script-update-success\",\"installId\":\"$INSTALL_ID\",\"from\":\"$INSTALLED_CHART_VERSION\",\"to\":\"$LATEST_CHART_VERSION\"}"

  download_latest_engine_image

  echo 'Done! (note that images could still be downloading in the background...)'
}

function main() {
  setup_http_utils
  report_status "{\"type\":\"install-script-init\",\"installId\":\"$INSTALL_ID\",\"uname\":\"$(uname -a)\"}"
  check_docker
  check_k3d
  get_latest_chart_version

  if k3d cluster list tensorleap &> /dev/null;
  then
    echo Detected existing tensorleap installation
    update_existing_chart
  else
    install_new_tensorleap_cluster
  fi
}

main

