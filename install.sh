set -euo pipefail

DISABLE_DOCKER_CHECKS=${DISABLE_DOCKER_CHECKS:=}
DISABLE_REPORTING=${DISABLE_REPORTING:=}
DISABLE_CLUSTER_CREATION=${DISABLE_CLUSTER_CREATION:=}
FILES_BRANCH=${FILES_BRANCH:=master}

DEFAULT_VOLUME="$HOME/tensorleap/data"
DATA_VOLUME=${DATA_VOLUME:=$DEFAULT_VOLUME:$DEFAULT_VOLUME}

INSTALL_ID=$RANDOM$RANDOM
DOCKER=docker
K3D=k3d
HELM=helm

VAR_DIR='/var/lib/tensorleap/standalone'

USE_LOCAL_HELM=${USE_LOCAL_HELM:=}

USE_GPU=${USE_GPU:=}
GPU_IMAGE='us-central1-docker.pkg.dev/tensorleap/main/k3s:v1.23.8-k3s1-cuda'

RETRIES=5
REQUEST_TIMEOUT=20
RETRY_DELAY=0

INSECURE=${INSECURE:=}
EXTRA_CURL_PARAMS=""
EXTRA_WGET_PARAMS=""
function setup_http_utils() {
  if [ "$INSECURE" == "true" ];
  then
    EXTRA_CURL_PARAMS="--insecure"
    EXTRA_WGET_PARAMS="--no-check-certificate"
  fi
  if type curl > /dev/null; then
    HTTP_GET="curl -sL --fail --connect-timeout $REQUEST_TIMEOUT --retry $RETRIES --retry-delay $RETRY_DELAY $EXTRA_CURL_PARAMS"
  elif type wget > /dev/null; then
    HTTP_GET="wget -q -O- --timeout=$REQUEST_TIMEOUT --tries=$RETRIES --wait=$RETRY_DELAY $EXTRA_WGET_PARAMS"
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
      curl -s --fail -XPOST -H 'Content-Type: application/json' $EXTRA_CURL_PARAMS $report_url -d "$1" &> /dev/null &
    elif type wget > /dev/null; then
      wget -q --method POST --header 'Content-Type: application/json' $EXTRA_WGET_PARAMS -O- --body-data "$1" $report_url &> /dev/null &
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
  if [ "$DISABLE_DOCKER_CHECKS" != "true" ]
  then
    NO_RESOURCES=''

    REQUIRED_MEMORY=6227000000
    REQUIRED_MEMORY_PRETTY=6Gb
    echo Checking docker memory limits...
    DOCKER_MEMORY=$($DOCKER info -f '{{json .MemTotal}}')
    DOCKER_MEMORY_PRETTY="$(($DOCKER_MEMORY /1024/1024/1024))Gb"
    echo "Docker has $DOCKER_MEMORY_PRETTY memory available."

    REQUIRED_STORAGE_KB=16777216
    REQUIRED_STORAGE_PRETTY=15Gb
    $DOCKER pull alpine 1> /dev/null
    DF_TMP_FILE=$(mktemp)
    echo Checking docker storage limits...
    $DOCKER run --rm -it alpine df -t overlay -P > $DF_TMP_FILE
    DF_OUTPUT=$(cat $DF_TMP_FILE | grep overlay | sed 's/  */:/g')
    DOCKER_TOTAL_STORAGE_KB=$(echo $DF_OUTPUT | cut -f2 -d:)
    DOCKER_TOTAL_STORAGE_PRETTY="$(($DOCKER_TOTAL_STORAGE_KB /1024/1024))Gb"
    DOCKER_FREE_STORAGE_KB=$(echo $DF_OUTPUT | cut -f4 -d:)
    DOCKER_FREE_STORAGE_PRETTY="$(($DOCKER_FREE_STORAGE_KB /1024/1024))Gb"
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
  fi
}

function check_image_in_registry() {
  local full_image_name=$1
  local image_name=${full_image_name/:*/}
  local registry_tags_url="127.0.0.1:5699/v2/${image_name#*/}/tags/list"
  local image_tag=${full_image_name/*:/}
  $HTTP_GET "$registry_tags_url" | grep "$image_tag" &> /dev/null
}

function cache_image() {
  local image=$1
  local image_repo=${image//\/*/}
  local image_name_without_repo=${image#*/}
  local target="127.0.0.1:5699/${image_name_without_repo}"
  local display_name=${image_name_without_repo/library\//}
  display_name=${display_name/main\//}
  if check_image_in_registry "$image";
  then
    echo "$display_name: Cached"
  else
    echo "$display_name: Pulling from $image_repo" && \
    $DOCKER pull -q "$image" > /dev/null && \
    $DOCKER tag "$image" "$target" > /dev/null && \
    echo "$display_name: Pushing to local registry" && \
    $DOCKER push "$target" > /dev/null && \
    echo "$display_name: Saved to cache"
  fi
}
export HTTP_GET
export DOCKER
export -f cache_image
export -f check_image_in_registry

function cache_images_in_registry() {
  if [ "$USE_GPU" == "true" ]
  then
    k3s_version=$(echo $GPU_IMAGE | sed 's/.*://;s/-cuda$//;s/-/+/')
  else
    k3s_version=$($K3D version | grep 'k3s version' | sed 's/.*version //;s/ .*//;s/-/+/')
  fi
  echo "Caching needed images in local registry..."
  cat \
    <($HTTP_GET https://raw.githubusercontent.com/tensorleap/helm-charts/$FILES_BRANCH/images.txt | grep -v 'engine') \
    <($HTTP_GET https://github.com/k3s-io/k3s/releases/download/$k3s_version/k3s-images.txt) \
    | xargs -P0 -IXXX bash -c "cache_image XXX"
}

function cache_engine_in_background() {
  local engine_image;
  engine_image=$($HTTP_GET https://raw.githubusercontent.com/tensorleap/helm-charts/$FILES_BRANCH/engine-latest-image);
  $DOCKER exec -d k3d-tensorleap-server-0 crictl pull "$engine_image"
}

function init_helm_values() {
  VOLUME_ENGINE_VALUES="localDataDirectory: ${DATA_VOLUME/*:/}"
  GPU_ENGINE_VALUES=""
  if [ "$USE_GPU" == "true" ]
  then
    GPU_ENGINE_VALUES='gpu: true'
  fi
}

function download_and_patch_k3d_cluster_config() {
  local sed_script="/volumes:/ a\\
\ \ - volume: $DATA_VOLUME\\
\ \ \ \ nodeFilters:\\
\ \ \ \ \ \ - server:*
"

  if [ "$USE_GPU" == "true" ]
  then
    sed_script="$sed_script;
/volumes:/ i\\
image: $GPU_IMAGE
;\$ a\\
\ \ runtime:\\
\ \ \ \ gpuRequest: all
"
  fi

  $HTTP_GET https://raw.githubusercontent.com/tensorleap/helm-charts/$FILES_BRANCH/config/k3d-config.yaml | \
    sed "$sed_script" \
    > $VAR_DIR/manifests/k3d-config.yaml
}

function create_helm_values_file() {
  echo "tensorleap-engine:
  ${VOLUME_ENGINE_VALUES}
  ${GPU_ENGINE_VALUES}" \
    > $VAR_DIR/manifests/helm-values.yaml

  echo --- > $VAR_DIR/manifests/tensorleap.yaml
}
function download_and_patch_helm_chart_manifest() {
  local sed_script="/targetNamespace:/ a\\
\ \ version: $LATEST_CHART_VERSION\\
\ \ valuesContent: |-\\
\ \ \ \ tensorleap-engine:\\
\ \ \ \ \ \ ${VOLUME_ENGINE_VALUES}\\
\ \ \ \ \ \ ${GPU_ENGINE_VALUES}
"
  $HTTP_GET https://raw.githubusercontent.com/tensorleap/helm-charts/$FILES_BRANCH/config/tensorleap.yaml | \
    sed "$sed_script" \
    > $VAR_DIR/manifests/tensorleap.yaml
}

function create_data_dir_if_needed() {
  local local_path=${DATA_VOLUME/:*/}
  [ -d "$local_path" ] || mkdir -p $local_path
}

function init_var_dir() {
  if [ ! -d "$VAR_DIR" ]; then
    sudo mkdir -p $VAR_DIR
    sudo chmod -R 777 $VAR_DIR
  fi

  [ -d "$VAR_DIR/manifests" ] || mkdir -p $VAR_DIR/manifests
  [ -d "$VAR_DIR/storage" ] || mkdir -p $VAR_DIR/storage
}

function create_config_files() {
  echo 'Downloading config files...'
  download_and_patch_k3d_cluster_config

  init_helm_values

  if [ "$USE_LOCAL_HELM" == "true" ]
  then
    create_helm_values_file
  else
    download_and_patch_helm_chart_manifest
  fi
}

function create_tensorleap_cluster() {
  if [ "$DISABLE_CLUSTER_CREATION" == "true" ]; then
    echo 'To continue installation run:'
    echo "$K3D cluster create --config $VAR_DIR/manifests/k3d-config.yaml"
    if [ "$USE_LOCAL_HELM" == "true" ]
    then
      echo $HELM repo add tensorleap https://helm.tensorleap.ai
      echo $HELM repo update tensorleap
      echo $HELM upgrade --install --create-namespace tensorleap tensorleap/tensorleap -n tensorleap \
      --values $VAR_DIR/manifests/helm-values.yaml \
      --wait
    fi
    exit 0;
  fi
  echo Creating tensorleap k3d cluster...
  report_status "{\"type\":\"install-script-creating-cluster\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\",\"volume\":\"$DATA_VOLUME\"}"
  $K3D cluster create --config $VAR_DIR/manifests/k3d-config.yaml

  if $K3D node list k3d-tensorleap-tools &> /dev/null;
  then
    echo Deleting temporary tools container...
    $K3D node delete k3d-tensorleap-tools
  fi
}

function run_helm_install() {
  echo Setting up helm repo...
  report_status "{\"type\":\"install-script-setting-helm-repo\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  $HELM repo add tensorleap https://helm.tensorleap.ai
  $HELM repo update tensorleap
  echo Running helm install...
  report_status "{\"type\":\"install-script-running-helm-upgrade\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  $HELM upgrade --install --create-namespace tensorleap tensorleap/tensorleap -n tensorleap \
    --values $VAR_DIR/manifests/helm-values.yaml \
    --wait
}

function wait_for_cluster_init() {
  if [ "$USE_LOCAL_HELM" != "true" ]; then
    echo 'Setting up cluster... (this may take a few minutes)'
    report_status "{\"type\":\"install-script-helm-install-wait\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"

    sleep 10 # wait for helm-install job to start
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
  if run_in_docker kubectl get -n kube-system HelmChart tensorleap > /dev/null;
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
    cache_engine_in_background
    exit 0
  fi
  echo Installed Version: $INSTALLED_CHART_VERSION
}

function install_new_tensorleap_cluster() {
  init_var_dir
  create_config_files
  cache_images_in_registry
  create_data_dir_if_needed
  create_tensorleap_cluster

  if [ "$USE_LOCAL_HELM" == "true" ]; then
    run_helm_install
  fi

  wait_for_cluster_init

  report_status "{\"type\":\"install-script-install-success\",\"installId\":\"$INSTALL_ID\",\"version\":\"$LATEST_CHART_VERSION\"}"
  echo "Congratulations! You've successfully installed Tensorleap"

  cache_engine_in_background
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
  cache_engine_in_background
}

function open_tensorleap_url() {
  sleep 2 # Avoid server-is-down

  TENSORLEAP_URL="http://127.0.0.1:4589"

  if [ "$OS_NAME" == "Linux" ];
  then
    if type xdg-open > /dev/null; then
      xdg-open $TENSORLEAP_URL 2> /dev/null
    elif type sensible-browser > /dev/null; then
      sensible-browser $TENSORLEAP_URL
    fi
  elif [ "$OS_NAME" == "Darwin" ];
  then
    if type open > /dev/null; then
      open $TENSORLEAP_URL
    fi
  fi

  echo "You can now access Tensorleap at $TENSORLEAP_URL"
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

  open_tensorleap_url
}

main

