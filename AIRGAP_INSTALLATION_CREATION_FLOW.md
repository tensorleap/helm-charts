# Airgap Installation Creation Flow

This guide provides a step-by-step process for creating a new Tensorleap airgap installation package.

## Prerequisites

Before starting, ensure:
- All desired code changes have been pushed to remote branches in their respective repositories (engine, node-server, web-ui)
- CI pipelines for building Docker images in ECR have completed successfully for all branches
- You have access to the helm-charts repository and GitHub Actions

## Step-by-Step Process

### 1. Update Image Tags in Values Files

Update the `image_tag` field in each of the following files to point to your desired branches:

- `charts/tensorleap/charts/engine/values.yaml`
- `charts/tensorleap/charts/node-server/values.yaml`
- `charts/tensorleap/charts/web-ui/values.yaml`

**Important Notes:**
- The branch name you specify must have been pushed to remote
- The CI pipeline for creating the Docker image in ECR must have completed successfully
- The image tag format is typically: `<branch-name>-<commit-hash>` (e.g., `master-8c86c5a5`)

**Example:**
```yaml
# In charts/tensorleap/charts/engine/values.yaml
image_tag: my-feature-branch-abc12345
```

### 2. Build Helm Charts and Update Images List

Run the following commands in sequence:

```bash
make build-helm
make update-images
```

**What these commands do:**
- `make build-helm`: Downloads helm dependencies and builds the chart packages
- `make update-images`: Extracts all image names from helm templates and updates `images.txt`

### 3. Push Your Branch to Remote

Commit your changes and push your helm-charts branch to the remote repository:

```bash
git add .
git commit -m "Update image tags for airgap release"
git push origin <your-branch-name>
```

### 4. Run Release Charts Workflow

1. Navigate to the helm-charts repository on GitHub
2. Go to **Actions** → **Release Charts**
3. Click **Run workflow**
4. Select your branch from the dropdown
5. Click **Run workflow**
6. Wait for the workflow to complete successfully

**What this does:**
- Publishes your helm charts to the chart repository
- Makes the charts available for installation

### 5. Run Release Installation Manifest Workflow

1. In GitHub Actions, go to **Release Installation Manifest**
2. Click **Run workflow**
3. Select your branch from the dropdown
4. Enter a custom tag prefix (optional but recommended for identification)
   - Example: `elbit-airgap`, `production`, `customer-name`
5. Click **Run workflow**
6. Wait for the workflow to complete

**What this does:**
- Creates a manifest file that points to your specific chart versions and image tags
- Creates a GitHub release with the tag format: `<custom-prefix>-<chart-version>`
- Example release tag: `elbit-airgap-1.4.74`

### 6. Copy the Release Tag

1. Go to the **Releases** section of the helm-charts repository
2. Find the release created by the previous workflow (it will have your custom prefix if you provided one)
3. Copy the full **release tag name** (not the release title)
   - Example: `manifest-1.4.74-elbit.0` or `elbit-airgap-manifest-1.4.74-elbit.0`

### 7. Run Release Airgap Pack Workflow

1. In GitHub Actions, go to **Release Airgap Pack**
2. Click **Run workflow**
3. Select your branch from the dropdown
4. In the **manifest_name** field, paste the release tag you copied in step 6
5. Click **Run workflow**
6. Wait for the workflow to complete (this may take some time as it packages all images)

**What this does:**
- Downloads all Docker images referenced in the manifest
- Creates a compressed airgap package (`.tar.gz`)
- Uploads the package to S3: `s3://tensorleap-assets/airgap-versions/tl-<manifest-tag>-linux-amd64.tar.gz`
- Updates the airgap versions index page

### 8. Verify the Release

After completion:
1. Check that the airgap package was uploaded to S3
2. Verify the package appears on the [latest airgap versions page](https://helm.tensorleap.ai/latest_airgap_versions.html)
3. The package can now be distributed to customers for airgap installation

## Troubleshooting

### Workflow Failures

**Release Charts fails:**
- Ensure your branch has been pushed to remote
- Check that helm chart versions are valid in `charts/tensorleap/Chart.yaml`

**Release Installation Manifest fails:**
- Verify that `charts/tensorleap/Chart.yaml` and `charts/tensorleap-infra/Chart.yaml` have valid versions
- Ensure the Release Charts workflow completed successfully first

**Release Airgap Pack fails:**
- Confirm the manifest tag name is correct (copy exactly from the release)
- Check AWS credentials are configured correctly in GitHub secrets
- Verify all image tags in values files point to existing images in ECR

### Image Not Found Errors

If you encounter image not found errors:
- Verify the branch was pushed to the correct repository
- Check that the CI pipeline completed successfully (go to the repository's Actions tab)
- Ensure the image tag format matches what's in ECR: `<branch-name>-<commit-hash>`

## Key Files Reference

- **Chart Versions**: `charts/tensorleap/Chart.yaml`, `charts/tensorleap-infra/Chart.yaml`
- **Image Tags**:
  - Engine: `charts/tensorleap/charts/engine/values.yaml`
  - Node Server: `charts/tensorleap/charts/node-server/values.yaml`
  - Web UI: `charts/tensorleap/charts/web-ui/values.yaml`
- **Makefile Targets**: `Makefile` (build-helm, update-images)
- **GitHub Workflows**: `.github/workflows/release_*.yml`

## Notes

- The airgap package includes all necessary Docker images, helm charts, and dependencies
- The final package size can be several GB depending on the images
- Customers will use the `leap server install -t <release-tag>` command to install from the airgap package
- Always test the installation in a staging environment before distributing to customers
