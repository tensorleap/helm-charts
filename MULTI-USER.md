# Using Tensorleap as multiple Linux users

One host, one install, several people operating it: whoever ran
`leap server install` (say `ubuntu`), a colleague, and the `ssm-user` account
AWS Session Manager logs you in as. The installer makes this work with one
local group and a fixed permission layout for its data dir. Nothing here needs
to be set up by hand; this page explains what the installer does and how to
check it.

## The `tensorleap` group

On every `leap server ...` command on Linux the installer:

1. creates the system group `tensorleap` if it is missing,
2. adds every human account on the host to it — accounts with a UID in the
   `login.defs` range and a login shell, plus `ssm-user` and whoever is running
   the command — and
3. if the current shell does not carry the group yet (membership is read at
   login), re-runs the same command under `sg tensorleap`, so the change takes
   effect immediately. Later logins have it natively.

Steps 1 and 2 need root, so the first run by a new user asks for sudo once.
Accounts created later (`ssm-user` appears on the first Session Manager login)
join on the next run of any command. To add someone by hand:

```bash
sudo usermod -aG tensorleap <user>
```

then have them log out and back in, or run `newgrp tensorleap`.

## Data dir layout and who may write

| Path (under `/var/lib/tensorleap/standalone`) | Mode | Written by |
|---|---|---|
| `.` (the data dir itself) | `root:tensorleap 2775` | the CLI, as any member |
| `manifests/`, `logs/`, `helm-cache/` | `:tensorleap 2775` | the CLI, as any member |
| `manifests/*.yaml`, `hostname`, log files, chart tarballs | `0664`, group `tensorleap` | the CLI |
| `manifests/kubeconfig.yaml` | `0660`, group `tensorleap` | the CLI; cluster-admin credentials, members only |
| `storage/*`, `registry/` | `0777` | the pods, with their own uids (hostPath volumes get no fsGroup) |
| `containerd/` | `root 0755` | the k3s node only; never copied on a transfer, rebuilt from the registry |

The `2775` directories carry the setgid bit, so everything created inside
inherits the group, and the CLI replaces its files by atomic rename rather than
opening them for writing. Together that is what lets a file `ubuntu` created be
updated by `ssm-user`. The installer re-applies this layout on every run, which
also migrates installs made by older versions (whose tree was world-writable).

## kubectl and helm for every member

The installer writes a shared kubeconfig into the data dir. Both `kubectl` and
`helm` honor `$KUBECONFIG`, so every member just needs two environment
variables pointing at it:

| Variable      | Value                                              | Why |
|---------------|----------------------------------------------------|-----|
| `TL_DATA_DIR` | `/var/lib/tensorleap/standalone` (or your `--data-dir`) | Where Tensorleap stores its data; the CLI reads this to find the install. |
| `KUBECONFIG`  | `$TL_DATA_DIR/manifests/kubeconfig.yaml`            | Shared kubeconfig. |

On **Linux** the installer drops `/etc/profile.d/tensorleap.sh` exporting both
`KUBECONFIG` and `TL_DATA_DIR` (the latter carries your actual `--data-dir`, so
other users find the install without re-passing the flag). On a fresh login
`kubectl` and `leap` just work, for every user, with no per-user setup. On
**mac** there is no equivalent system-wide drop-in, so add the exports to your
shell rc. The manual steps below are only needed on mac, or if that file is
missing:

```bash
sudo tee /etc/profile.d/tensorleap.sh >/dev/null <<'EOT'
export TL_DATA_DIR=/var/lib/tensorleap/standalone
export KUBECONFIG=$TL_DATA_DIR/manifests/kubeconfig.yaml
EOT
sudo chmod 644 /etc/profile.d/tensorleap.sh
```

Log out and back in (or `source /etc/profile.d/tensorleap.sh`), then verify:

```bash
echo $KUBECONFIG
kubectl get nodes
```

The kubeconfig is readable by group members only. A user who is not in the
group gets `permission denied` from `kubectl` until they are added.

## Docker

The CLI talks to Docker directly, so each user also needs Docker access:
membership of the `docker` group (`sudo usermod -aG docker <user>`), or running
the CLI with `sudo`. Root needs neither group.

## Moving the data dir

`--data-dir` to a new location moves the storage, registry, manifests and logs
with ownership and modes preserved. The `containerd/` image cache is dropped
rather than moved: it is a derivative of the local registry, so the install
that follows re-pulls every image from there, and gigabytes of root-owned
image layers never go through a copy that could lose their ownership.

## When pods fail with `permission denied` inside their own image

A container image cache that was copied or restored without preserving
ownership (`cp -r`, some backup tools) ends up root-owned throughout, and every
container that runs as non-root then fails writing inside its own image —
ingress-nginx logs
`could not create PEM certificate file ... permission denied`, for one. The
state survives pod restarts and reboots because every new pod stacks on the
same layers.

The installer checks for this signature (a snapshot tree without a single
non-root file):

- `install` and `reinstall` rebuild the cache automatically before creating
  the cluster; images are re-pulled from the local registry, application data
  is untouched.
- `run` and `upgrade` cannot rebuild it under a live cluster, so they print a
  warning that points to `leap server reinstall`.

## Testing the group setup

The unit tests cover the policy without touching the host. The account-database
parts (`groupadd`, `usermod`, the `sg` re-exec) run against a real Linux host
only when `TL_SHARED_GROUP_INTEGRATION=1`, which changes that host's groups, so
use a throwaway container:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.24 bash -ec '
  apt-get update -qq && apt-get install -y -qq sudo >/dev/null
  useradd -m -s /bin/bash alice && useradd -m -s /bin/bash bob && useradd -m -s /bin/bash carol && useradd -r -s /usr/sbin/nologin svc
  echo "alice ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/alice
  chmod -R a+rwX /go
  go test ./pkg/local/
  run() { su "$1" -c "cd /src && HOME=/home/$1 GOCACHE=/home/$1/.cache/go-build GOMODCACHE=/go/pkg/mod PATH=/usr/local/go/bin:\$PATH TL_SHARED_GROUP_INTEGRATION=1 TL_DATA_DIR=/var/lib/tensorleap/standalone $2 go test -run \"$3\" -v ./pkg/local/"; }
  # alice, sudo-capable, first run on a fresh host: creates the group, adds
  # everyone, re-executes under sg, creates the data dir
  run alice "TL_SHARED_GROUP_EXPECT_REEXEC=1 TL_SHARED_GROUP_EXPECT_MEMBERS=\"alice bob carol\" TL_SHARED_GROUP_EXPECT_ABSENT=svc" "TestSharedGroupEndToEnd|TestSharedDataDirEndToEnd"
  # bob, a member without sudo whose shell started after he was added: no
  # sudo, no re-exec, replaces the file alice wrote
  run bob "" "TestSharedGroupEndToEnd|TestSharedDataDirEndToEnd"
  # carol, removed from the group and without sudo: cannot be added, gets
  # the actionable error
  gpasswd -d carol tensorleap
  run carol "TL_SHARED_GROUP_EXPECT_ERROR=1" "TestSharedGroupEndToEnd"
  # root (the CLI under sudo): heals membership, is not added itself
  TL_SHARED_GROUP_INTEGRATION=1 TL_SHARED_GROUP_EXPECT_MEMBERS="alice bob carol" TL_SHARED_GROUP_EXPECT_ABSENT="svc root" go test -run TestSharedGroupEndToEnd -v ./pkg/local/
'
```
