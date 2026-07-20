package handler

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/logic/workspace"
	"github.com/luoxiaojun1992/data-agent/internal/service/artifact"
)

// ArtifactHandler provides HTTP handlers for artifact and workspace operations.
type ArtifactHandler struct {
	storage artifact.StorageService
	wm      *workspace.Manager
}

// NewArtifactHandler creates a new HTTP handler.
func NewArtifactHandler(storage artifact.StorageService, wm *workspace.Manager) *ArtifactHandler {
	return &ArtifactHandler{storage: storage, wm: wm}
}

// Upload handles multipart file upload.
func (h *ArtifactHandler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	defer file.Close()

	userID, _ := c.Get("user_id")
	sessionID := c.PostForm("session_id")
	persistent := c.PostForm("persistent") == "true"
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	art, err := h.storage.Upload(
		userID.(string), sessionID, "",
		header.Filename, mimeType, file, persistent,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, art)
}

// Download handles file download.
func (h *ArtifactHandler) Download(c *gin.Context) {
	artifactID := c.Param("id")

	art, err := h.storage.FindByID(artifactID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	data, err := h.storage.Download(artifactID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", art.MimeType)
	c.Header("Content-Disposition", "attachment; filename=\""+art.Name+"\"")
	c.Data(http.StatusOK, art.MimeType, data)
}

// Delete removes an artifact (idempotent).
func (h *ArtifactHandler) Delete(c *gin.Context) {
	artifactID := c.Param("id")
	if err := h.storage.Delete(artifactID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": artifactID})
}

// ListSession returns all artifacts for a session.
func (h *ArtifactHandler) ListSession(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id query parameter required"})
		return
	}

	artifacts, err := h.storage.ListBySession(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, artifacts)
}

// ListWorkspace lists workspace files for a session.
func (h *ArtifactHandler) ListWorkspace(c *gin.Context) {
	sessionID := c.Param("session_id")
	userID, _ := c.Get("user_id")

	files, err := h.wm.List(userID.(string), sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"session_id": sessionID, "files": files})
}

// ReadWorkspaceFile reads a file from the workspace.
func (h *ArtifactHandler) ReadWorkspaceFile(c *gin.Context) {
	sessionID := c.Param("session_id")
	filename := c.Param("filename")
	userID, _ := c.Get("user_id")

	data, err := h.wm.ReadFile(userID.(string), sessionID, filename)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/octet-stream", data)
}

// WriteWorkspaceFile writes a file to the workspace.
func (h *ArtifactHandler) WriteWorkspaceFile(c *gin.Context) {
	sessionID := c.Param("session_id")
	filename := c.Param("filename")
	userID, _ := c.Get("user_id")

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read body"})
		return
	}

	if err := h.wm.WriteFile(userID.(string), sessionID, filename, data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "written", "filename": filename})
}
