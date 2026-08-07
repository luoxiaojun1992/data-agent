package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/service/apicollection"
)

type APICollectionHandler struct {
	svc *apicollection.Service
}

func NewAPICollectionHandler(svc *apicollection.Service) *APICollectionHandler {
	return &APICollectionHandler{svc: svc}
}

// List returns paginated API collections.
// Admin: only own. System_admin: all.
func (h *APICollectionHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")
	role := c.GetString("role")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	ownerID := ""
	if role != "system_admin" {
		ownerID = userID
	}

	result, err := h.svc.List(c.Request.Context(), ownerID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// Create uploads a new OpenAPI collection.
func (h *APICollectionHandler) Create(c *gin.Context) {
	userID := c.GetString("user_id")
	name := c.PostForm("name")
	description := c.PostForm("description")

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer f.Close()

	rawSpec := make([]byte, file.Size)
	f.Read(rawSpec)

	coll, err := h.svc.CreateUpload(c.Request.Context(), userID, name, description, rawSpec, "")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, coll)
}

// Get returns collection details.
func (h *APICollectionHandler) Get(c *gin.Context) {
	userID := c.GetString("user_id")
	role := c.GetString("role")
	id := c.Param("id")

	ownerID := ""
	if role != "system_admin" {
		ownerID = userID
	}

	coll, err := h.svc.Get(c.Request.Context(), id, ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, coll)
}

// Update modifies name/description.
func (h *APICollectionHandler) Update(c *gin.Context) {
	userID := c.GetString("user_id")
	id := c.Param("id")
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.Update(c.Request.Context(), id, userID, req.Name, req.Description); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// Delete removes a collection.
func (h *APICollectionHandler) Delete(c *gin.Context) {
	userID := c.GetString("user_id")
	id := c.Param("id")
	if err := h.svc.Delete(c.Request.Context(), id, userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// Approve sets the approval status.
func (h *APICollectionHandler) Approve(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Status model.APICollectionStatus `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Status != model.APICollectionApproved && req.Status != model.APICollectionRejected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be 'approved' or 'rejected'"})
		return
	}
	if err := h.svc.Approve(c.Request.Context(), id, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": string(req.Status)})
}
