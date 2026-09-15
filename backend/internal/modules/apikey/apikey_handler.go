package apikey

import (
	"encoding/json"
	"erent/internal/dto/request"
	"erent/internal/dto/response"
	jwtservice "erent/internal/middleware/jwt"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type ApikeyHandler struct {
	service *ApikeyService
}

func NewHandler(service *ApikeyService) *ApikeyHandler { return &ApikeyHandler{service: service} }

func (h *ApikeyHandler) UpdateApikey(c *gin.Context) {
	userid, id, ok := apikeyRequestIDs(c)
	if !ok {
		return
	}
	var req request.UpdateApikeyRequest
	if err := c.ShouldBindJSON(&req); err != nil || (req.Disabled == nil && len(req.ExpiresAt) == 0) {
		response.Error(c, http.StatusBadRequest, 40000, "invalid api key request")
		return
	}
	var expiresAt *time.Time
	if len(req.ExpiresAt) != 0 {
		if err := json.Unmarshal(req.ExpiresAt, &expiresAt); err != nil {
			response.Error(c, http.StatusBadRequest, 40000, "invalid expires_at")
			return
		}
	}
	apikeyMutationResponse(c, h.service.UpdateApikey(c.Request.Context(), userid, id, req.Disabled, expiresAt, len(req.ExpiresAt) != 0))
}

func (h *ApikeyHandler) ReplaceApikeyAccounts(c *gin.Context) {
	userid, id, ok := apikeyRequestIDs(c)
	if !ok {
		return
	}
	var req request.CreatApikeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, 40000, "invalid api key request")
		return
	}
	apikeyMutationResponse(c, h.service.ReplaceApikeyAccounts(c.Request.Context(), userid, id, req.OAuthInfo))
}

func (h *ApikeyHandler) DeleteApikey(c *gin.Context) {
	userid, id, ok := apikeyRequestIDs(c)
	if !ok {
		return
	}
	apikeyMutationResponse(c, h.service.DeleteApikey(c.Request.Context(), userid, id))
}

func apikeyRequestIDs(c *gin.Context) (uint64, uint64, bool) {
	value, _ := c.Get(jwtservice.UserIDContextKey)
	userid, ok := value.(uint64)
	if !ok || userid == 0 || userid > 1<<63-1 {
		response.Error(c, http.StatusUnauthorized, 40100, "invalid user identity")
		return 0, 0, false
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 63)
	if err != nil || id == 0 {
		response.Error(c, http.StatusBadRequest, 40000, "invalid api key id")
		return 0, 0, false
	}
	return userid, id, true
}

func apikeyMutationResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrApikeyNotFound):
		response.Error(c, http.StatusNotFound, 40400, ErrApikeyNotFound.Error())
	case errors.Is(err, ErrInvalidAccounts):
		response.Error(c, http.StatusBadRequest, 40000, ErrInvalidAccounts.Error())
	case err != nil:
		response.Error(c, http.StatusInternalServerError, 50000, "internal server error")
	default:
		response.Success(c, nil)
	}
}

func (h *ApikeyHandler) CreatApikey(c *gin.Context) {
	value, _ := c.Get(jwtservice.UserIDContextKey)
	userid, ok := value.(uint64)
	if !ok || userid == 0 {
		response.Error(c, http.StatusUnauthorized, 40100, "invalid user identity")
		return
	}
	var req request.CreatApikeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, 40000, "invalid api key request")
		return
	}
	raw, err := h.service.CreatApikey(c.Request.Context(), userid, req.OAuthInfo)
	switch {
	case errors.Is(err, ErrInvalidAccounts):
		response.Error(c, http.StatusBadRequest, 40000, ErrInvalidAccounts.Error())
	case err != nil:
		response.Error(c, http.StatusInternalServerError, 50000, "internal server error")
	default:
		c.Header("Cache-Control", "no-store")
		response.SuccessWithStatus(c, http.StatusCreated, gin.H{"api_key": raw}, "success")
	}
}

func (h *ApikeyHandler) ApikeyList(c *gin.Context) {
	value, _ := c.Get(jwtservice.UserIDContextKey)
	userid, ok := value.(uint64)
	if !ok || userid == 0 {
		response.Error(c, http.StatusUnauthorized, 40100, "invalid user identity")
		return
	}
	items, err := h.service.UserApikeyList(c.Request.Context(), userid)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, 50000, "internal server error")
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, gin.H{"apikeylist": items})
}
