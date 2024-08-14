<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Airgap Versions</title>
</head>
<body>
  <h1>Tensorleap Helm Charts</h1>
  <p>this repo contains the helm charts for tensorleap</p>
  <h1>Airgap Versions</h1>
  <ul id="file-list"></ul>

  <script>
    async function fetchFiles() {
      const url = 'https://tensorleap-assets.s3.amazonaws.com/?list-type=2&prefix=airgap-versions/';
      const response = await fetch(url);
      const xmlText = await response.text();
      const parser = new DOMParser();
      const xmlDoc = parser.parseFromString(xmlText, 'application/xml');

      const files = xmlDoc.getElementsByTagName('Contents');
      const fileList = document.getElementById('file-list');

      for (let i = 0; i < files.length; i++) {
        const key = files[i].getElementsByTagName('Key')[0].textContent;
        const lastModified = files[i].getElementsByTagName('LastModified')[0].textContent;
        const fileUrl = `https://tensorleap-assets.s3.amazonaws.com/${key}`;

        const listItem = document.createElement('li');
        const link = document.createElement('a');
        link.href = fileUrl;
        link.textContent = key;
        link.download = key;

        listItem.appendChild(link);
        listItem.append(` - ${lastModified}`);
        fileList.appendChild(listItem);
      }
    }

    fetchFiles();
  </script>
</body>
</html>
