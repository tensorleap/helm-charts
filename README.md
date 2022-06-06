# Helm Test Repository

To install tensorleap on your local machine run:

```
bash <(curl -s https://helm.tensorleap.ai/install.sh)
```

After about 10 minutes, tensorleap will be available on port `4589`. \
The cluster is available in `kubectx` with the name `k3d-tensorleap`. (don't forget to switch to `tensorleap` namespace). \
To see the `helm` installation output run `kubectl logs -f -n kube-system job/helm-install-tensorleap`.
