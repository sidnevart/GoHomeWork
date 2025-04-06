package api

import (
	"net/http"

	"a.sidnev/internal/docker"
)

func NewRouter() *http.ServeMux {
	dockerClient := docker.NewClient()
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/image-download-size", methodGuard(ImageDownloadSizeHandler(dockerClient)))
	mux.HandleFunc("/api/v1/os-release-info", methodGuard(OSReleaseInfoHandler(dockerClient)))

	return mux
}

func methodGuard(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		handler(w, r)
	}
}