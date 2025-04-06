package docker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

func (c *Client) GetImageLayerInfo(repo, name, tag string) (int, int64, error) {
	manifest, err := c.fetchManifest(repo, name, tag)
	if err != nil {
		log.Printf("ERROR: Failed to get manifest for layer info %s/%s:%s: %v", repo, name, tag, err)
		return 0, 0, err
	}

	uniqueDigests := make(map[string]struct{})
	var totalSize int64
	for _, layer := range manifest.Layers {
		if _, exists := uniqueDigests[layer.Digest]; !exists {
			uniqueDigests[layer.Digest] = struct{}{}
			totalSize += layer.Size
		}
	}

	log.Printf("INFO: Layer info for %s/%s:%s: %d layers, %d bytes", repo, name, tag, len(uniqueDigests), totalSize)
	return len(uniqueDigests), totalSize, nil
}

func (c *Client) GetOSReleaseInfo(repo, name, tag string) (*OSRelease, error) {
	client := &http.Client{
		Timeout: 10 * time.Minute,
	}

	manifest, err := c.fetchManifest(repo, name, tag)
	if err != nil {
		log.Printf("ERROR: Failed to get manifest for OS release info %s/%s:%s: %v", repo, name, tag, err)
		return nil, err
	}

	token, err := c.getAuthToken(repo, name)
	if err != nil {
		log.Printf("ERROR: Failed to get auth token for blobs %s/%s:%s: %v", repo, name, tag, err)
		return nil, ErrInternal
	}

	for i, layer := range manifest.Layers {
		log.Printf("INFO: Fetching blob %s for %s/%s:%s (layer %d/%d)", layer.Digest, repo, name, tag, i+1, len(manifest.Layers))
		req, err := http.NewRequest("GET", c.blobURL(repo, name, layer.Digest), nil)
		if err != nil {
			log.Printf("ERROR: Failed to create blob request %s for %s/%s:%s: %v", layer.Digest, repo, name, tag, err)
			return nil, ErrInternal
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("ERROR: Failed to fetch blob %s for %s/%s:%s: %v", layer.Digest, repo, name, tag, err)
			return nil, ErrInternal
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("ERROR: Unexpected status %d for blob %s in %s/%s:%s", resp.StatusCode, layer.Digest, repo, name, tag)
			return nil, ErrInternal
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("ERROR: Failed to read blob body %s for %s/%s:%s: %v", layer.Digest, repo, name, tag, err)
			return nil, ErrInternal
		}
		log.Printf("INFO: Successfully read blob %s for %s/%s:%s, size: %d bytes", layer.Digest, repo, name, tag, len(body))

		tr := tar.NewReader(bytes.NewReader(body))
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("ERROR: Failed to read tar stream for %s in %s/%s:%s: %v", layer.Digest, repo, name, tag, err)
				break
			}

			log.Printf("INFO: Found file %s in layer %s for %s/%s:%s", hdr.Name, layer.Digest, repo, name, tag)
			if hdr.Name == "etc/os-release" || hdr.Name == "usr/lib/os-release" {
				osRelease, err := parseOSRelease(tr)
				if err != nil {
					log.Printf("ERROR: Failed to parse os-release for %s/%s:%s: %v", repo, name, tag, err)
					return nil, err
				}
				log.Printf("INFO: Found os-release for %s/%s:%s: %+v", repo, name, tag, osRelease)
				return osRelease, nil
			}
		}
	}
	log.Printf("INFO: os-release not found in any layer for %s/%s:%s", repo, name, tag)
	return nil, ErrNotFound
}

func (c *Client) fetchManifest(repo, name, tag string) (*Manifest, error) {
	fullName := name
	if repo == "registry-1.docker.io" && !strings.Contains(name, "/") {
		fullName = "library/" + name
	}

	req, err := http.NewRequest("GET", c.manifestURL(repo, fullName, tag), nil)
	if err != nil {
		log.Printf("ERROR: Failed to create manifest request for %s/%s:%s: %v", repo, fullName, tag, err)
		return nil, ErrInternal
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("ERROR: Failed to fetch manifest for %s/%s:%s: %v", repo, fullName, tag, err)
		return nil, ErrInternal
	}

	token := ""
	for attempt := 1; attempt <= 2; attempt++ {
		if resp.StatusCode == http.StatusUnauthorized {
			log.Printf("INFO: Received 401 for %s/%s:%s, WWW-Authenticate: %s", repo, fullName, tag, resp.Header.Get("WWW-Authenticate"))
			resp.Body.Close()
			token, err = c.getAuthToken(repo, name)
			if err != nil {
				log.Printf("ERROR: Failed to get auth token for %s/%s:%s: %v", repo, fullName, tag, err)
				return nil, ErrInternal
			}
			log.Printf("INFO: Retrying manifest request (attempt %d) with token for %s/%s:%s", attempt, repo, fullName, tag)

			req, err = http.NewRequest("GET", c.manifestURL(repo, fullName, tag), nil)
			if err != nil {
				log.Printf("ERROR: Failed to create retry manifest request for %s/%s:%s: %v", repo, fullName, tag, err)
				return nil, ErrInternal
			}
			req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err = c.httpClient.Do(req)
			if err != nil {
				log.Printf("ERROR: Failed to fetch manifest with token for %s/%s:%s: %v", repo, fullName, tag, err)
				return nil, ErrInternal
			}
			continue
		}
		break
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		log.Printf("ERROR: Failed to read response body for %s/%s:%s: %v", repo, fullName, tag, err)
		return nil, ErrInternal
	}

	log.Printf("INFO: Manifest response for %s/%s:%s: status=%d, body=%s", repo, fullName, tag, resp.StatusCode, string(body))

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("INFO: Manifest not found for %s/%s:%s", repo, fullName, tag)
		return nil, ErrNotFound
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		log.Printf("ERROR: Rate limit exceeded for %s/%s:%s, headers: %+v", repo, fullName, tag, resp.Header)
		return nil, ErrInternal
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: Unexpected status %d for manifest %s/%s:%s, body: %s, headers: %+v", resp.StatusCode, repo, fullName, tag, string(body), resp.Header)
		return nil, ErrInternal
	}

	var manifestList struct {
		SchemaVersion int    `json:"schemaVersion"`
		MediaType     string `json:"mediaType"`
		Manifests     []struct {
			MediaType string `json:"mediaType"`
			Digest    string `json:"digest"`
			Platform  struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if err := json.Unmarshal(body, &manifestList); err == nil && manifestList.MediaType == "application/vnd.oci.image.index.v1+json" {
		log.Printf("INFO: Received manifest list for %s/%s:%s, selecting amd64 manifest", repo, fullName, tag)
		for _, m := range manifestList.Manifests {
			if m.Platform.Architecture == "amd64" && m.Platform.OS == "linux" && m.MediaType == "application/vnd.oci.image.manifest.v1+json" {
				req, err = http.NewRequest("GET", c.manifestURL(repo, fullName, m.Digest), nil)
				if err != nil {
					log.Printf("ERROR: Failed to create manifest request for digest %s: %v", m.Digest, err)
					return nil, ErrInternal
				}
				req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
				if token == "" {
					token, _ = c.getAuthToken(repo, name)
				}
				req.Header.Set("Authorization", "Bearer "+token)

				resp, err = c.httpClient.Do(req)
				if err != nil {
					log.Printf("ERROR: Failed to fetch manifest digest %s: %v", m.Digest, err)
					return nil, ErrInternal
				}
				defer resp.Body.Close()

				body, err = io.ReadAll(resp.Body)
				if err != nil {
					log.Printf("ERROR: Failed to read manifest digest body for %s/%s:%s: %v", repo, fullName, tag, err)
					return nil, ErrInternal
				}
				log.Printf("INFO: Manifest digest response for %s/%s:%s: status=%d, body=%s", repo, fullName, tag, resp.StatusCode, string(body))

				if resp.StatusCode != http.StatusOK {
					log.Printf("ERROR: Unexpected status %d for manifest digest %s/%s:%s, body: %s", resp.StatusCode, repo, fullName, tag, string(body))
					return nil, ErrInternal
				}
				break
			}
		}
	}

	var manifest Manifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		log.Printf("ERROR: Failed to parse manifest for %s/%s:%s: %v, body: %s", repo, fullName, tag, err, string(body))
		return nil, ErrInternal
	}

	log.Printf("INFO: Parsed manifest for %s/%s:%s: %d layers", repo, fullName, tag, len(manifest.Layers))
	return &manifest, nil
}

func (c *Client) getAuthToken(repo, name string) (string, error) {
	if repo == "docker.io" {
		repo = "registry-1.docker.io"
	}
	fullName := name
	if repo == "registry-1.docker.io" && !strings.Contains(name, "/") {
		fullName = "library/" + name
	}

	authURL := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", fullName)
	resp, err := c.httpClient.Get(authURL)
	if err != nil {
		log.Printf("ERROR: Failed to fetch auth token for %s: %v", fullName, err)
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("ERROR: Failed to read auth token response body for %s: %v", fullName, err)
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: Unexpected status %d from auth endpoint for %s, body: %s", resp.StatusCode, fullName, string(body))
		return "", fmt.Errorf("unexpected status %d from auth endpoint", resp.StatusCode)
	}

	var authResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &authResp); err != nil {
		log.Printf("ERROR: Failed to parse auth token response for %s: %v, body: %s", fullName, err, string(body))
		return "", err
	}
	if authResp.Token == "" {
		log.Printf("ERROR: Empty token received for %s, body: %s", fullName, string(body))
		return "", fmt.Errorf("empty token received")
	}
	log.Printf("INFO: Successfully fetched auth token for %s: %s", fullName, authResp.Token[:20]+"...")
	return authResp.Token, nil
}

func parseOSRelease(r io.Reader) (*OSRelease, error) {
	osr := &OSRelease{}
	buf := make([]byte, 512)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("empty file")
	}

	scanner := bufio.NewScanner(bytes.NewReader(buf[:n]))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := parts[0], strings.Trim(strings.TrimSpace(parts[1]), `"`)
		switch key {
		case "PRETTY_NAME":
			osr.PrettyName = value
		case "NAME":
			osr.Name = value
		case "VERSION_ID":
			osr.VersionID = value
		case "ID":
			osr.ID = value
		case "HOME_URL":
			osr.HomeURL = value
		}
	}
	if osr.ID == "" {
		return nil, fmt.Errorf("os-release ID not found")
	}
	return osr, nil
}
