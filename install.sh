set -euo pipefail

DISABLE_REPORTING=${DISABLE_REPORTING:=}
DISABLE_CLUSTER_CREATION=${DISABLE_CLUSTER_CREATION:=}
FILES_BRANCH=${FILES_BRANCH:=master}

INSTALL_ID=$RANDOM$RANDOM
DOCKER=docker
K3D=k3d
HELM=helm

VAR_DIR='/var/lib/tensorleap/standalone'

REGISTRY_PORT=${TENSORLEAP_REGISTRY_PORT:=5699}

FIX_DNS=${FIX_DNS:=}
USE_LOCAL_HELM=${USE_LOCAL_HELM:=}

USE_GPU=${USE_GPU:=}
GPU_IMAGE='us-central1-docker.pkg.dev/tensorleap/main/k3s:v1.23.8-k3s1-cuda'

RETRIES=5
REQUEST_TIMEOUT=20
RETRY_DELAY=0
function setup_http_utils() {
  if type curl > /dev/null; then
    HTTP_GET="curl -sL --fail --connect-timeout $REQUEST_TIMEOUT --retry $RETRIES --retry-delay $RETRY_DELAY"
  elif type wget > /dev/null; then
    HTTP_GET="wget -q -O- --timeout=$REQUEST_TIMEOUT --tries=$RETRIES --wait=$RETRY_DELAY"
  else
    echo you must have either curl or wget installed.
    exit -1
  fi
}

function download_file() {
  if type curl > /dev/null; then
    $HTTP_GET $1 '-o' $2
  elif type wget > /dev/null; then
    $HTTP_GET '-O' $2 $1
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
  if !($K3D version);
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
    HELM='sudo helm'
  fi

  REQUIRED_DOCKER_MAJOR_VERSION=20
  DOCKER_VERSION=$($DOCKER version | sed -ne '/Client:/,/^$/ p' | grep '^ Version:' | xargs | cut -d ' ' -f 2)
  DOCKER_MAJOR_VERSION=$(echo $DOCKER_VERSION | cut -d '.' -f 1)

  if [ $DOCKER_MAJOR_VERSION -lt $REQUIRED_DOCKER_MAJOR_VERSION ];
  then
    report_status "{\"type\":\"install-script-old-docker-version\",\"installId\":\"$INSTALL_ID\",\"dockerVersion\":\"$DOCKER_VERSION\"}"
    echo "Tensorleap standalone installation requires docker version $REQUIRED_DOCKER_MAJOR_VERSION or above. Installed version: $DOCKER_VERSION"
      exit -1
  fi
}

function check_helm() {
  if [ "$USE_LOCAL_HELM" == "true" ]
  then
    echo Checking helm installation
    if !($HELM version);
    then
      report_status "{\"type\":\"install-script-helm-not-installed\",\"installId\":\"$INSTALL_ID\"}"
      echo Please install helm!
      exit -1
    fi
  fi
}

function get_latest_chart_version() {
  echo Getting latest version...
  LATEST_CHART_VERSION=$($HTTP_GET https://raw.githubusercontent.com/tensorleap/helm-charts/$FILES_BRANCH/charts/tensorleap/Chart.yaml | grep '^version:' | cut -c 10-)
  echo $LATEST_CHART_VERSION
}

function run_in_docker() {
  $DOCKER exec -it k3d-tensorleap-server-0 $*
}

function check_docker_requirements() {

  NO_RESOURCES=''

  REQUIRED_MEMORY=6227000000
  REQUIRED_MEMORY_PRETTY=6Gb
  echo Checking docker memory limits...
  DOCKER_MEMORY=$($DOCKER info -f '{{json .MemTotal}}')
  DOCKER_MEMORY_PRETTY="$(echo "scale=2; $DOCKER_MEMORY /1024/1024/1024" | bc -l)Gb"
  echo "Docker has $DOCKER_MEMORY_PRETTY memory available."

  REQUIRED_STORAGE_KB=16777216
  REQUIRED_STORAGE_PRETTY=15Gb
  $DOCKER pull alpine 1> /dev/null
  DF_TMP_FILE=$(mktemp)
  echo Checking docker storage limits...
  $DOCKER run --rm -it alpine df -t overlay -P > $DF_TMP_FILE
  DF_OUTPUT=$(cat $DF_TMP_FILE | grep overlay | sed 's/  */:/g')
  DOCKER_TOTAL_STORAGE_KB=$(echo $DF_OUTPUT | cut -f2 -d:)
  DOCKER_TOTAL_STORAGE_PRETTY="$(echo "scale=2; $DOCKER_TOTAL_STORAGE_KB /1024/1024" | bc -l)Gb"
  DOCKER_FREE_STORAGE_KB=$(echo $DF_OUTPUT | cut -f4 -d:)
  DOCKER_FREE_STORAGE_PRETTY="$(echo "scale=2; $DOCKER_FREE_STORAGE_KB /1024/1024" | bc -l)Gb"
  echo "Docker has $DOCKER_FREE_STORAGE_PRETTY free storage available ($DOCKER_TOTAL_STORAGE_PRETTY total)."

  if [ $DOCKER_MEMORY -lt $REQUIRED_MEMORY ];
  then
    echo "Please increase docker memory limit to at least $REQUIRED_MEMORY_PRETTY"
    NO_RESOURCES=true
  fi

  if [ $DOCKER_FREE_STORAGE_KB -lt $REQUIRED_STORAGE_KB ];
  then
    echo "Please increase docker storage limit, tensorleap required at least $REQUIRED_STORAGE_PRETTY free storage"
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
  if [ "$USE_GPU" == "true" ]
  then
    k3s_version=$(echo $GPU_IMAGE | sed 's/.*://;s/-cuda$//;s/-/+/')
  else
    k3s_version=$($K3D version | grep 'k3s version' | sed 's/.*version //;s/ .*//;s/-/+/')
  fi
  cat \
    <($HTTP_GET https://raw.githubusercontent.com/tensorleap/helm-charts/$FILES_BRANCH/images.txt) \
    <($HTTP_GET https://github.com/k3s-io/k3s/releases/download/$k3s_version/k3s-images.txt) \
    | xargs -P3 -IXXX bash -c "cache_image $REGISTRY_PORT XXX"
}

function get_installation_options() {
  DEFAULT_VOLUME="$HOME/tensorleap/data"
  VOLUME=${DATA_VOLUME:=$DEFAULT_VOLUME:$DEFAULT_VOLUME}
  LOCAL_PATH=${VOLUME/:*/}
  [ ! -d "$LOCAL_PATH" ] && mkdir -p $LOCAL_PATH

  VOLUME_ENGINE_VALUES="localDataDirectory: ${VOLUME/*:/}"
  VOLUMES_MOUNT_PARAM="-v $VOLUME@server:*"

  if [ "$FIX_DNS" == "true" ]
  then
    VOLUMES_MOUNT_PARAM="$VOLUMES_MOUNT_PARAM -v /etc/resolv.conf:/etc/resolv.conf@server:*"
  fi

  GPU_CLUSTER_PARAMS=""
  GPU_ENGINE_VALUES=""
  if [ "$USE_GPU" == "true" ]
  then
    GPU_CLUSTER_PARAMS="--image $GPU_IMAGE --gpus all"
    GPU_ENGINE_VALUES='gpu: true'
  fi

  VALUES_FILE=$(cat << EOF
tensorleap-engine:
  ${VOLUME_ENGINE_VALUES}
  ${GPU_ENGINE_VALUES}
EOF
)

  VALUES_CONTENT=$(cat << EOF
  valuesContent: |-
    tensorleap-engine:
      ${VOLUME_ENGINE_VALUES}
      ${GPU_ENGINE_VALUES}
EOF
)
}

function init_var_dir() {
  sudo mkdir -p $VAR_DIR
  sudo chmod -R 777 $VAR_DIR
  mkdir -p $VAR_DIR/manifests
  mkdir -p $VAR_DIR/storage
  mkdir -p $VAR_DIR/scripts

  echo 'Downloading config files...'
  download_file https://raw.githubusercontent.com/tensorleap/helm-charts/$FILES_BRANCH/config/k3d-config.yaml $VAR_DIR/manifests/k3d-config.yaml
  download_file https://raw.githubusercontent.com/tensorleap/helm-charts/$FILES_BRANCH/config/k3d-entrypoint.sh $VAR_DIR/scripts/k3d-entrypoint.sh
  sudo chmod +x $VAR_DIR/scripts/k3d-entrypoint.sh
}

function create_tensorleap_helm_manifest() {
  if [ "$USE_LOCAL_HELM" == "true" ]
  then
    echo "$VALUES_FILE" > $VAR_DIR/manifests/helm-values.yaml
    echo --- > $VAR_DIR/manifests/tensorleap.yaml
  else
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
  fi
}

function create_tensorleap_cluster() {
  if [ "$DISABLE_CLUSTER_CREATION" == "true" ]; then
    echo 'To continue installation run:'
    echo "$K3D cluster create --config $VAR_DIR/manifests/k3d-config.yaml $GPU_CLUSTER_PARAMS $VOLUMES_MOUNT_PARAM"
    exit 0;
  fi
  echo Creating tensorleap k3d cluster...
  report_status "{\"type\":\"install-script-creating-cluster\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\",\"volume\":\"$VOLUME\"}"
  $K3D cluster create --config $VAR_DIR/manifests/k3d-config.yaml $GPU_CLUSTER_PARAMS $VOLUMES_MOUNT_PARAM \
    2>&1 | grep -v 'ERRO.*/bin/k3d-entrypoint\.sh' # This hides the expected warning about k3d-entrypoint replacement
}

function run_helm_install() {
  if [ "$USE_LOCAL_HELM" == "true" ]; then
    echo Setting up helm repo...
    report_status "{\"type\":\"install-script-setting-helm-repo\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
    $HELM repo add tensorleap https://helm.tensorleap.ai
    $HELM repo update tensorleap
    echo Running helm install...
    report_status "{\"type\":\"install-script-running-helm-upgrade\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
    $HELM upgrade --install --create-namespace tensorleap tensorleap/tensorleap -n tensorleap \
      --values $VAR_DIR/manifests/helm-values.yaml \
      --wait
  fi
}

function wait_for_cluster_init() {
  if [ "$USE_LOCAL_HELM" != "true" ]; then
    echo 'Setting up cluster... (this may take a few minutes)'
    report_status "{\"type\":\"install-script-helm-install-wait\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
    if !(run_in_docker kubectl wait --for=condition=complete --timeout=25m -n kube-system job helm-install-tensorleap);
    then
      report_status "{\"type\":\"install-script-helm-install-timeout\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
      echo "Timeout! Cluster is starting in the background, wait a few minutes and see if Tensorleap is available on http://127.0.0.1:4589 If it's not, contact support"
      exit -1
    fi
  fi
  echo 'Waiting for containers to initialize... (Just a few more minutes!)'
  report_status "{\"type\":\"install-script-deployment-wait\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  if !(run_in_docker kubectl wait --for=condition=available --timeout=25m -n tensorleap deploy -l app.kubernetes.io/managed-by=Helm);
  then
    report_status "{\"type\":\"install-script-deployment-timeout\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
    echo "Timeout! Cluster is starting in the background, wait a few minutes and see if Tensorleap is available on http://127.0.0.1:4589 If it's not, contact support"
    exit -1
  fi
}

function check_installed_version() {
  if run_in_docker kubectl get -n kube-system HelmChart tensorleap;
  then
    INSTALLED_CHART_VERSION=$(run_in_docker kubectl get -n kube-system HelmChart tensorleap -o jsonpath='{.spec.version}')
    USE_LOCAL_HELM=false
  else
    INSTALLED_CHART_VERSION=$(helm list -n tensorleap -a -f tensorleap -o yaml | grep 'chart:' | sed 's/.*tensorleap-//')
    USE_LOCAL_HELM=true
  fi

  if [ "$LATEST_CHART_VERSION" == "$INSTALLED_CHART_VERSION" ]
  then
    report_status "{\"type\":\"install-script-up-to-date\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\",\"localHelm\":\"$USE_LOCAL_HELM\"}"
    echo Installation in up to date!
    exit 0
  fi
  echo Installed Version: $INSTALLED_CHART_VERSION
}

function install_new_tensorleap_cluster() {
  get_installation_options
  create_docker_registry
  cache_images_in_registry
  init_var_dir
  create_tensorleap_helm_manifest
  create_tensorleap_cluster
  run_helm_install
  wait_for_cluster_init

  report_status "{\"type\":\"install-script-install-success\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  echo "Tensorleap demo installed! It should be available now on http://127.0.0.1:4589"
}

function update_existing_chart() {
  cache_images_in_registry
  check_installed_version

  report_status "{\"type\":\"install-script-update-started\",\"installId\":\"$INSTALL_ID\",\"from\":\"$INSTALLED_CHART_VERSION\",\"to\":\"$LATEST_CHART_VERSION\",\"localHelm\":\"$USE_LOCAL_HELM\"}"

  echo Updating to latest version...
  if [ "$USE_LOCAL_HELM" == "true" ]; then
    run_helm_install
  else
    run_in_docker kubectl patch -n kube-system  HelmChart/tensorleap --type='merge' -p "{\"spec\":{\"version\":\"$LATEST_CHART_VERSION\"}}"
  fi
  report_status "{\"type\":\"install-script-update-success\",\"installId\":\"$INSTALL_ID\",\"from\":\"$INSTALLED_CHART_VERSION\",\"to\":\"$LATEST_CHART_VERSION\"}"

  echo 'Done! (note that images could still be downloading in the background...)'
}

function main() {
  echo Please note that during the installation you may be required to provide your computer password to enable communication with the docker.
  setup_http_utils
  report_status "{\"type\":\"install-script-init\",\"installId\":\"$INSTALL_ID\",\"uname\":\"$(uname -a)\"}"
  check_docker
  check_k3d
  check_helm
  get_latest_chart_version

  if $K3D cluster list tensorleap &> /dev/null;
  then
    echo Detected existing tensorleap installation
    update_existing_chart
  else
    install_new_tensorleap_cluster
  fi
}

main

