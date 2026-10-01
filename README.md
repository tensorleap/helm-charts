# Helm-charts
This repo hold the charts and installer for standalone installation

# Airgap installation
Show [here](https://helm.tensorleap.ai/latest_airgap_versions.html) the latest Airgap versions

## Creating a Custom Release in Helm-Charts

**1. Create a New Branch:**
   Start by creating a new branch in this repository. This branch will host your custom changes.

**2. Update Image Tags:**
   Change the image tags for each image you want to modify. The image tag is located in three files. The easiest way is to replace the current tag with the new image tag - is to repleace the name of the current tag with the new image tag. To find your image tag, navigate to `mark-stable`` -> `set stable tag`` on the branch where you made the changes.

**3. Update Chart Version:**
   Open the Tensorleap chart file (`charts/tensorleap/chart.yaml`). Update the version, for example, `0.0.140` to `0.0.141-[beta|alpha|custom-word].0`. When updating the version, update the last digit (`.0` to `.1`).

**4. Publish Helm Chart Workflow:**
   Go to the GitHub Actions on the Helm-Charts repository and run the `Release Charts` workflow on your branch. This workflow publishes the Helm chart from your branch.

**5. Generate Custom Manifest Release:**
   After the previous workflow completes, go to the GitHub Actions on the Helm-Charts repository. Run the `Release Installation Manifest` workflow on your branch and, before running it, input a custom prefix. This workflow creates a JSON file pointing to the images and Helm chart location.

**6. Install or Upgrade Custom Release:**
   Your custom release is now ready to be installed. Visit the releases section of the `helm-charts` repository to find a release name that starts with the custom name you provided earlier. Copy the release name and use one of the following commands:
   - `leap server install -t [release-name]`
   - `leap server upgrade -t [release-name]`
## Resetting a forgotten password

Passwords are stored in Keycloak and there is no email-based reset. An operator
resets them from the machine where the server is installed:

```bash
leap server reset-password user@example.com
```

It sets a temporary password (printed once), signs the user out everywhere, and
Keycloak forces a new password at the next browser login. `leap auth login -u/-p`
works again after that. Existing API keys are unaffected.

Without `leap server` (plain helm install), run the same steps with kubectl:

```bash
kubectl -n tensorleap exec -i keycloak-0 -- bash -s <<'SCRIPT'
set -euo pipefail
KC=/opt/keycloak/bin/kcadm.sh; CFG=/tmp/kcadm.config
"$KC" config credentials --config "$CFG" --server http://localhost:8080/auth --realm master --user "$KEYCLOAK_ADMIN" --password "$KEYCLOAK_ADMIN_PASSWORD" >/dev/null
ID=$("$KC" get users --config "$CFG" -r tensorleap -q email=user@example.com -q exact=true --fields id --format csv --noquotes)
"$KC" set-password --config "$CFG" -r tensorleap --userid "$ID" --new-password 'Temp-Passw0rd' --temporary
"$KC" create users/"$ID"/logout --config "$CFG" -r tensorleap
SCRIPT
```
