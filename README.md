# Helm-charts
This repo hold the charts and installer for standalone installation



## Creating a Custom Release in Helm-Charts

**1. Create a New Branch:**
   Start by creating a new branch in this repository. This branch will host your custom changes.

**2. Update Image Tags:**
   Change the image tags for each image you want to modify. The image tag is located in three files. The easiest way is to replace the current tag with the new image tag - is to repleace the name of the current tag with the new image tag. To find your image tag, navigate to `mark-stable`` -> `set stable tag`` on the branch where you made the changes.

**3. Update Chart Version:**
   Open the TensorFlow chart file (`charts/tensorflow/chart.yaml`). Update the version from, for example, `0.0.140` to `0.0.141-[beta|alpha|custom-word].0`. When creating a new version, update the last digit (`.0` to `.1`).

**4. Publish Helm Chart Workflow:**
   Go to the GitHub Actions on the Helm-Charts repository and run the `Release Charts` workflow on your branch. This workflow publishes the Helm chart from your branch.

**5. Generate Custom Manifest Release:**
   After the previous workflow completes, go to the GitHub Actions on the Helm-Charts repository. Run the `Release Installation Manifest` workflow on your branch and, before running it, input a custom prefix. This workflow creates a JSON file pointing to the images and Helm chart location.

**6. Install or Upgrade Custom Release:**
   Your custom release is now ready to be installed. Visit the releases section of the `helm-charts` repository to find a release name that starts with the custom name you provided earlier. Copy the release name and use one of the following commands:
   - `leap server install -t [release-name]`
   - `leap server upgrade -t [release-name]`