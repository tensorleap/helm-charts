# Using Tensorleap as multiple Linux users

By default `leap server install` writes the cluster kubeconfig into the
**installing user's** `~/.kube/config`. A second user on the same machine has no
kubeconfig, so their `kubectl` / `helm` can't reach the cluster.

Since the installer also writes a **shared** kubeconfig into the data dir, every
user just needs two environment variables pointing at it. Set them once,
system-wide, and `kubectl`/`helm` work for everyone.

## The two variables

| Variable      | Value                                              | Why |
|---------------|----------------------------------------------------|-----|
| `TL_DATA_DIR` | `/var/lib/tensorleap/standalone` (or your `--data-dir`) | Where Tensorleap stores its data; the CLI reads this to find the install. |
| `KUBECONFIG`  | `$TL_DATA_DIR/manifests/kubeconfig.yaml`            | Shared kubeconfig. Both `kubectl` and `helm` honor `$KUBECONFIG`. |

The installer writes the shared kubeconfig on both Linux and mac. On **Linux**
it also drops `/etc/profile.d/tensorleap-kubeconfig.sh` exporting `KUBECONFIG`,
so on a fresh login `kubectl` usually just works. On **mac** there's no
equivalent system-wide drop-in, so add the export to your shell rc (see
[Per-user](#per-user-alternative) below). The manual steps are also useful if
that file is missing, if you installed to a custom `--data-dir`, or to make the
setup explicit.

## Set it system-wide (recommended)

Create one profile script that every login shell sources:

```bash
sudo tee /etc/profile.d/tensorleap.sh >/dev/null <<'EOF'
export TL_DATA_DIR=/var/lib/tensorleap/standalone
export KUBECONFIG=$TL_DATA_DIR/manifests/kubeconfig.yaml
EOF
sudo chmod 644 /etc/profile.d/tensorleap.sh
```

Log out and back in (or `source /etc/profile.d/tensorleap.sh` in your current
shell), then verify:

```bash
echo $KUBECONFIG
kubectl get nodes
```

> If you installed to a custom data dir, replace
> `/var/lib/tensorleap/standalone` with that path.

## Per-user alternative

If you can't touch `/etc`, add the same two lines to each user's `~/.bashrc`
(or `~/.zshrc`):

```bash
echo 'export TL_DATA_DIR=/var/lib/tensorleap/standalone' >> ~/.bashrc
echo 'export KUBECONFIG=$TL_DATA_DIR/manifests/kubeconfig.yaml' >> ~/.bashrc
source ~/.bashrc
```

## Security note

The kubeconfig carries **cluster-admin** credentials, and the shared file is
world-accessible (`777`, matching the data dir). On a single-node local dev box
this matches the existing trust model (the data dir is already world-writable).
If your box needs
tighter isolation, restrict the file to a shared group instead:

```bash
sudo groupadd -f tensorleap
sudo usermod -aG tensorleap <each-user>
sudo chown root:tensorleap $KUBECONFIG
sudo chmod 640 $KUBECONFIG
```
