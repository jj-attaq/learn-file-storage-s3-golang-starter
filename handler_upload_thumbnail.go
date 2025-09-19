package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	// TODO: implement the upload here
	const maxMemory = 10 << 20 // 10 MB
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse multi part form", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type for thumbnail", nil)
		return
	}

	// fileData, err := io.ReadAll(file)
	// if err != nil {
	// 	respondWithError(w, http.StatusInternalServerError, "Error reading file", err)
	// 	return
	// }

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't find video", err)
		return
	} else if userID != video.UserID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", err)
		return
	}

	// bas64Encoded := base64.StdEncoding.EncodeToString(fileData)
	// base64DataURL := fmt.Sprintf("data:%s;base64,%s", mediaType, bas64Encoded)
	// video.ThumbnailURL = &base64DataURL
	// fmt.Sprintf("/assets/%s.%s", videoID.String(), mediaType)

	parts := strings.SplitN(mediaType, "/", 2)
	if len(parts) != 2 { /* handle error */
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", nil)
		return
	}
	extension := parts[1]
	if extension == "jpeg" {
		extension = "jpg"
	}
	if strings.Contains(extension, "+") { // e.g. image/svg+xml -> svg+xml
		extension = strings.SplitN(extension, "+", 2)[0]
	}

	diskPath := filepath.Join(cfg.assetsRoot, fmt.Sprintf("%s.%s", videoID.String(), extension))

	storageFile, err := os.Create(diskPath)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unable to save video", err)
		return
	}
	defer storageFile.Close()

	if _, err := io.Copy(storageFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to save thumbnail", err)
		return
	}

	url := fmt.Sprintf("http://localhost:%s/assets/%s.%s", cfg.port, videoID.String(), extension)

	video.ThumbnailURL = &url

	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
