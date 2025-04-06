package api

import (
	"encoding/json"
	"net/http"

	"a.sidnev/internal/docker"
)

type ImageRequest struct {
	Repository string `json:"repository"`
	Name       string `json:"name"`
	Tag        string `json:"tag"`
}

type ImageSizeResponse struct {
	LayersCount int   `json:"layers_count"`
	TotalSize   int64 `json:"total_size"`
}

type OSReleaseResponse struct {
	PrettyName string `json:"pretty_name"`
	Name       string `json:"name"`
	VersionID  string `json:"version_id"`
	ID         string `json:"id"`
	HomeURL    string `json:"home_url"`
}

func ImageDownloadSizeHandler(dockerClient *docker.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ImageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Repository == "" || req.Name == "" {
			http.Error(w, "Missing repository or name", http.StatusBadRequest)
			return
		}
		if req.Tag == "" {
			req.Tag = "latest"
		}

		layersCount, totalSize, err := dockerClient.GetImageLayerInfo(req.Repository, req.Name, req.Tag)
		if err != nil {
			if err == docker.ErrNotFound {
				http.Error(w, "Image not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		resp := ImageSizeResponse{
			LayersCount: layersCount,
			TotalSize:   totalSize,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

func OSReleaseInfoHandler(dockerClient *docker.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ImageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Repository == "" || req.Name == "" {
			http.Error(w, "Missing repository or name", http.StatusBadRequest)
			return
		}
		if req.Tag == "" {
			req.Tag = "latest"
		}

		osRelease, err := dockerClient.GetOSReleaseInfo(req.Repository, req.Name, req.Tag)
		if err != nil {
			if err == docker.ErrNotFound {
				http.Error(w, "OS release info not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		resp := OSReleaseResponse{
			PrettyName: osRelease.PrettyName,
			Name:       osRelease.Name,
			VersionID:  osRelease.VersionID,
			ID:         osRelease.ID,
			HomeURL:    osRelease.HomeURL,
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}
