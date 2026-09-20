package externalapikey

import "github.com/gin-gonic/gin"

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

//TODO
func CreatExternalApiKey(c *gin.Context) {

}

//TODO
func DeleteExternalApiKey(c *gin.Context) {

}

//TODO
func UpdateExternalApiKey(c *gin.Context) {

}

//TODO
func List(c *gin.Context) {

}
