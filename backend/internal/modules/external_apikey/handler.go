package externalapikey

import (
	"erent/internal/dto/request"
	"erent/internal/dto/response"
	jwtservice "erent/internal/middleware/jwt"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func requestIDs(c *gin.Context, withID bool) (uint64, uint64, bool) {
	value, _ := c.Get(jwtservice.UserIDContextKey)
	userID, ok := value.(uint64)
	if !ok || userID == 0 || userID > 1<<63-1 {
		response.Error(c, http.StatusUnauthorized, 40100, "invalid user identity")
		return 0, 0, false
	}
	if !withID {
		return userID, 0, true
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 63)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, 40000, "invalid external api key id")
		return 0, 0, false
	}
	return userID, id, true
}

func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		response.Error(c, http.StatusBadRequest, 40000, ErrInvalidRequest.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.Error(c, http.StatusNotFound, 40400, "external api key not found")
	default:
		response.Error(c, http.StatusInternalServerError, 50000, "internal server error")
	}
}

func (h *Handler) CreatExternalApiKey(c *gin.Context) {
	userID, _, ok := requestIDs(c, false)
	if !ok {
		return
	}
	var req request.ExternalApikeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, ErrInvalidRequest)
		return
	}
	item, err := h.service.Create(c.Request.Context(), userID, req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.SuccessWithStatus(c, http.StatusCreated, item, "success")
}

func (h *Handler) DeleteExternalApiKey(c *gin.Context) {
	userID, id, ok := requestIDs(c, true)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), userID, id); err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, nil)
}

// UpdateExternalApiKey replaces all mutable fields, including empty suffixes.
func (h *Handler) UpdateExternalApiKey(c *gin.Context) {
	userID, id, ok := requestIDs(c, true)
	if !ok {
		return
	}
	var req request.ExternalApikeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, ErrInvalidRequest)
		return
	}
	if err := h.service.Update(c.Request.Context(), userID, id, req); err != nil {
		respondError(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *Handler) List(c *gin.Context) {
	userID, _, ok := requestIDs(c, false)
	if !ok {
		return
	}
	items, err := h.service.List(c.Request.Context(), userID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"apikeylist": items})
}
