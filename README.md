# Tensorleap Helm Charts

To install tensorleap on your local machine run:

```
bash <(curl -s https://helm.tensorleap.ai/install.sh)
```
# or
```
bash <(wget -O- https://helm.tensorleap.ai/install.sh)
```

After about 10 minutes, tensorleap will be available on port `4589`. \
The cluster is available in `kubectx` with the name `k3d-tensorleap`. (don't forget to switch to `tensorleap` namespace). \
To see the `helm` installation output run `kubectl logs -f -n kube-system job/helm-install-tensorleap`.

## Uninstall

<details>
<summary>You can completly uninstall tensorleap with these instuctions</summary>

```bash
# Delete main cluster
k3d cluster delete tensorleap

# Remove docker registry with cached images
k3d registry delete tensorleap-registry

# Remove docker internal cache
docker system prune --volumes --all --force

# Remove configuration and state files
rm -rf /var/lib/tensorleap/standalone
```

</details>

## Manual Installation

<details>
<summary>If you don't want to run the install script for some reason, you can still install Tensorleap manually by taking the following steps</summary>

1. Install [docker](https://docs.docker.com/get-docker/) and [k3d](https://k3d.io/v5.4.6/#installation).
2. Create the needed directory structure:

```bash
VAR_DIR='/var/lib/tensorleap/standalone'
sudo mkdir -p $VAR_DIR
sudo chmod -R 777 $VAR_DIR
mkdir -p $VAR_DIR/manifests
mkdir -p $VAR_DIR/storage
mkdir -p $VAR_DIR/scripts
```

3. Download configuration files from the [config](./config) folder in this repo:

```bash
VAR_DIR='/var/lib/tensorleap/standalone'
BASE_CONFIG_URL='https://raw.githubusercontent.com/tensorleap/helm-charts/master/config'
curl $BASE_CONFIG_URL/k3d-config.yaml -o $VAR_DIR/manifests/k3d-config.yaml
curl $BASE_CONFIG_URL/tensorleap.yaml -o $VAR_DIR/manifests/tensorleap.yaml
curl $BASE_CONFIG_URL/k3d-entrypoint.sh -o $VAR_DIR/scripts/k3d-entrypoint.sh
chmod +x $VAR_DIR/scripts/k3d-entrypoint.sh
```

4. (optional) Setup volume mounts by updating the configuration files:

```yaml
# /var/lib/tensorleap/standalone/manifests/k3d-config.yaml
volumes:
  - volume: ...
  - volume: ...
  - volume: path/on/host:path/inside/container

# /var/lib/tensorleap/standalone/manifests/tensorleap.yaml
spec:
  ...
  valuesContent: |-
    tensorleap-engine:
      localDataDirectory: /path/inside/container
```

5. (optional) Enable experimental GPU support:
   1. Make sure to have [nvidia drivers](https://docs.nvidia.com/datacenter/tesla/tesla-installation-notes/index.html#ubuntu-lts) installed and configured to [work with docker](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html#installing-on-ubuntu-and-debian).
   2. Update the configuration files:

```yaml
# /var/lib/tensorleap/standalone/manifests/k3d-config.yaml
image: us-central1-docker.pkg.dev/tensorleap/main/k3s:v1.23.8-k3s1-cuda
options:
  runtime:
    gpuRequest: all

# /var/lib/tensorleap/standalone/manifests/tensorleap.yaml
spec:
  ...
  valuesContent: |-
    tensorleap-engine:
      gpu: true
```

6. Create a local docker registry (running in a container):

```bash
k3d registry create tensorleap-registry -p 5699
```

7. (optional): Pull the images listed in [images.txt](./images.txt), and push them to the local repository. This will allow faster startup and offline usage.

```
for image in $(curl https://raw.githubusercontent.com/tensorleap/helm-charts/master/images.txt);
do
  target=$(echo $image | sed "s/[^\/]*\//127.0.0.1:5699\//" | sed 's/@.*$//')
  docker pull $image && \
  docker tag $image $target && \
  docker push $target
done
```

8. Create a cluster with tensorleap installed:

```bash
k3d cluster create --config /var/lib/tensorleap/standalone/manifests/k3d-config.yaml
```

note that it will take some time for the installation to be ready. \
You can monitor progress by inspecting the cluster with `kubectl`

9. After the self initialization of the cluster, Tensorleap should be available on http://127.0.0.1:4589
</details>
