package chat

import (
	"context"
	"encoding/json"
	"erent/internal/dto/request"
	"erent/internal/dto/response"
	"erent/internal/modules/apikey"
	"erent/internal/modules/chat/route"
	"erent/internal/modules/translator"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ChatHandler struct {
	service *ChatService
}

func NewChatHandler(service *ChatService) *ChatHandler {
	return &ChatHandler{service: service}
}

func (h *ChatHandler) Chat(c *gin.Context, w http.ResponseWriter, clientformat translator.Format) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.Error(c, http.StatusBadRequest, 400, "invalid request body")
		return
	}
	var req request.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Model) == "" {
		response.Error(c, http.StatusBadRequest, 400, "invalid chat request")
		return
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	value, _ := c.Get("apikey_accounts")
	accounts, _ := value.([]apikey.ApikeyAccountItem)
	selected, err := apikey.GetCredential(ctx, accounts)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, apikey.ErrOauthForbidden) {
			code = http.StatusForbidden
		} else if errors.Is(err, apikey.ErrCredentialUnavailable) {
			code = http.StatusServiceUnavailable
		}
		response.Error(c, code, code*100, "unable to obtain upstream credential")
		return
	}
	credential := Credential{Mode: route.AuthMode(selected.Mode), Secret: selected.Secret}
	if req.Stream {
		respchan, errchan := h.service.ChatStream(ctx, body, credential, clientformat, req.Model)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		started := false
		for respchan != nil || errchan != nil {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-respchan:
				if !ok {
					// respCh 已关闭
					respchan = nil
					continue
				}

				if len(data) == 0 {
					continue
				}
				if !started {
					w.Header().Set("Content-Type", "text/event-stream")
					w.Header().Set("Cache-Control", "no-cache")
				}
				if _, err := w.Write(data); err != nil {
					return
				}
				flusher.Flush()
				started = true

			case err, ok := <-errchan:
				if !ok {
					// errCh 已关闭
					errchan = nil
					continue
				}
				if err == nil {
					continue
				}
				if started == false {
					httpStatus := http.StatusBadGateway
					switch {
					case errors.Is(err, errors.ErrUnsupported):
						httpStatus = http.StatusBadRequest
					default:
						switch status.Code(err) {
						case codes.InvalidArgument:
							httpStatus = http.StatusBadRequest
						case codes.Unauthenticated:
							httpStatus = http.StatusUnauthorized
						case codes.PermissionDenied:
							httpStatus = http.StatusForbidden
						case codes.ResourceExhausted:
							httpStatus = http.StatusTooManyRequests
						case codes.Unavailable:
							httpStatus = http.StatusServiceUnavailable
						case codes.DeadlineExceeded:
							httpStatus = http.StatusGatewayTimeout
						}
					}
					response.Error(c, httpStatus, httpStatus, "chat stream failed")
					return
				}
				payload := gin.H{"error": gin.H{"type": "api_error", "message": "upstream stream failed"}}
				if clientformat == translator.FormatClaude {
					payload["type"] = "error"
				}
				if clientformat == translator.FormatOpenAIResponse {
					payload = gin.H{"type": "error", "code": "server_error", "message": "upstream stream failed", "param": nil}
				}
				errorJSON, _ := json.Marshal(payload)
				if clientformat != translator.FormatOpenAI {
					if _, err := fmt.Fprint(w, "event: error\n"); err != nil {
						return
					}
				}
				if _, err := fmt.Fprintf(w, "data: %s\n\n", errorJSON); err != nil {
					return
				}
				flusher.Flush()
				return
			}
		}

	}
	if !req.Stream {
		result, err := h.service.ChatNonStream(ctx, body, credential, clientformat, req.Model)
		if err != nil {
			httpStatus := http.StatusBadGateway
			if errors.Is(err, errors.ErrUnsupported) {
				httpStatus = http.StatusBadRequest
			} else {
				switch status.Code(err) {
				case codes.InvalidArgument:
					httpStatus = http.StatusBadRequest
				case codes.Unauthenticated:
					httpStatus = http.StatusUnauthorized
				case codes.PermissionDenied:
					httpStatus = http.StatusForbidden
				case codes.ResourceExhausted:
					httpStatus = http.StatusTooManyRequests
				case codes.Unavailable:
					httpStatus = http.StatusServiceUnavailable
				case codes.DeadlineExceeded:
					httpStatus = http.StatusGatewayTimeout
				}
			}
			response.Error(c, httpStatus, httpStatus, "chat request failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, result.Content)
	}
}
