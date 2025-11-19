package github

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Release struct {
	Name    string `json:"name"`
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name        string `json:"name"`
		DownloadUrl string `json:"browser_download_url"`
	} `json:"assets"`
}

func GetReleasesPage(owner, repo string, page, per_page int) ([]Release, error) {

	if page < 1 {
		page = 1
	}
	if per_page < 1 {
		per_page = 1
	} else if per_page > 100 {
		per_page = 100
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?page=%v&per_page=%v", owner, repo, page, per_page)

	var releases []Release

	res, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	err = json.NewDecoder(res.Body).Decode(&releases)
	if err != nil {
		return nil, err
	}

	return releases, nil
}

func GetTagArtifact(owner, repo, fileName, tag string) ([]byte, error) {
	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, tag, fileName)

	return getFileByUrl(url)
}

func getFileByUrl(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed getting file from (%s): %v", url, resp.StatusCode)
	}
	imagesFile, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return imagesFile, nil
}

func GetFileContent(owner, repo, filePath, ref string) ([]byte, error) {

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, filePath, ref)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to retrieve file: %s, status: %s", url, resp.Status)
	}

	var fileData struct {
		Content string `json:"content"`
	}
	err = json.NewDecoder(resp.Body).Decode(&fileData)
	if err != nil {
		return nil, err
	}

	decodedContent, err := base64.StdEncoding.DecodeString(fileData.Content)
	if err != nil {
		return nil, err
	}

	return decodedContent, nil
}
