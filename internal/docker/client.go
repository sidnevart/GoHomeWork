package docker

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) manifestURL(repo, name, reference string) string {
	if repo == "docker.io" || repo == "dockerhub.timeweb.cloud" {
		repo = "registry-1.docker.io"
	}

	if repo == "registry-1.docker.io" && !strings.Contains(name, "/") {
		name = "library/" + name
	}

	return fmt.Sprintf("https://%s/v2/%s/manifests/%s", repo, name, reference)
}

func (c *Client) blobURL(repo, name, digest string) string {
	if repo == "docker.io" || repo == "dockerhub.timeweb.cloud" {
		repo = "registry-1.docker.io" 
	}

	if repo == "registry-1.docker.io" && !strings.Contains(name, "/") {
		name = "library/" + name
	}

	return fmt.Sprintf("https://%s/v2/%s/blobs/%s", repo, name, digest)
}
