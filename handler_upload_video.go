package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const uploadLimit = 1 << 30 // 1GB
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)

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

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't find video", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Not authorized to update this video", nil)
		return
	}

	file, handler, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType, _, err := mime.ParseMediaType(handler.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Content-Type", err)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid file type, only MP4 is allowed", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not create temp file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not write file to disk", err)
		return
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Could not reset file pointer", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error getting aspect ratio", err)
		return
	}

	var prefix string
	switch {
	case aspectRatio == "16:9":
		prefix = "landscape"
	case aspectRatio == "9:16":
		prefix = "portrait"
	case aspectRatio == "other":
		prefix = "other"
	}

	key := prefix + "/" + getAssetPath(mediaType)
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(key),
		Body:        tempFile,
		ContentType: aws.String(mediaType),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error uploading file to S3", err)
		return
	}

	url := cfg.getObjectURL(key)
	video.VideoURL = &url
	if err := cfg.db.UpdateVideo(video); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}

func getVideoAspectRatio(filePath string) (string, error) {
	// ffprobe -v error -print_format json -show_streams PATH_TO_VIDEO
	type parameters struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Width     int    `json:"width,omitempty"`
			Height    int    `json:"height,omitempty"`
		}
	}
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	buf := bytes.Buffer{}
	cmd.Stdout = &buf
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return "", err
	}

	decoder := json.NewDecoder(&buf)
	params := parameters{}

	err := decoder.Decode(&params)
	if err != nil {
		return "", err
	}

	if len(params.Streams) < 1 {
		return "", errors.New("ffprobe did not output any streams")
	}

	// math
	var width, height int
	ok := false
	for i, stream := range params.Streams {
		if stream.CodecType == "video" {
			width = params.Streams[i].Width
			height = params.Streams[i].Height
			ok = true
			break
		}
	}
	if !ok {
		return "", errors.New("ffprobe did not find video stream")
	}

	if width == 0 || height == 0 {
		return "", errors.New("width and height can't be 0")
	}

	gcd := findGCD(width, height)
	widthRatio, heightRatio := width/gcd, height/gcd
	switch {
	case widthRatio == 16 && heightRatio == 9:
		return "16:9", nil
	case widthRatio == 9 && heightRatio == 16:
		return "9:16", nil
	default:
		r := float64(width) / float64(height)
		if math.Abs(r-(9.0/16.0)) < 0.02 {
			return "9:16", nil
		}
		if math.Abs(r-(16.0/9.0)) < 0.02 {
			return "16:9", nil
		}
		return "other", nil
	}
}

// https://www.geeksforgeeks.org/dsa/euclidean-algorithms-basic-and-extended/
func findGCD(a, b int) int {
	if a == 0 {
		return b
	}
	return findGCD(b%a, a)
}
