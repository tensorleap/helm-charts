const https = require("https");
const fs = require("fs");

const bucket = "tensorleap-assets";
const prefix = "airgap-versions/";

const url = `https://${bucket}.s3.amazonaws.com/?list-type=2&prefix=${prefix}`;
const fileName = "latest_airgap_versions.md";
const filterFilePrefix = "tl-manifest";
const latestVersion = 50;

https
  .get(url, (res) => {
    let data = "";

    res.on("data", (chunk) => {
      data += chunk;
    });

    res.on("end", () => {
      const files = [];
      const parser =
        /<Key>(.*?)<\/Key>.*?<LastModified>(.*?)<\/LastModified>/gs;
      let match;

      while ((match = parser.exec(data)) !== null) {
        const path = match[1];
        const fileUrl = `https://${bucket}.s3.amazonaws.com/${path}`;
        const key = path.slice(prefix.length);
        if (!key.startsWith(filterFilePrefix)) {
          continue;
        }
        files.push({ key, fileUrl, lastModified: match[2] });
      }

      // Sort files by LastModified date (newest first)
      files.sort((a, b) => new Date(b.lastModified) - new Date(a.lastModified));

      const latestFiles = files.slice(0, latestVersion);

      let markdownContent = `# Latest ${latestVersion} Airgap Versions\n\n`;
      markdownContent += `| File Name | Last Modified |\n`;
      markdownContent += `|-----------|---------------|\n`;

      latestFiles.forEach((file) => {
        const { key, fileUrl, lastModified } = file;
        markdownContent += `| [${key}](${fileUrl}) | ${lastModified} |\n`;
      });

      fs.writeFileSync(fileName, markdownContent);
      console.log("Markdown file created successfully.");
    });
  })
  .on("error", (e) => {
    console.error(`Error fetching data: ${e.message}`);
  });
