package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/gbplantwiki/gbplantwiki/internal/constants"
	"github.com/gbplantwiki/gbplantwiki/internal/dto"
	"github.com/gbplantwiki/gbplantwiki/internal/middleware"
	"github.com/gbplantwiki/gbplantwiki/internal/service"
	"github.com/gbplantwiki/gbplantwiki/internal/util"
)

// PlantMergeHandler exposes the admin plant merge console endpoints.
type PlantMergeHandler struct {
	svc    *service.PlantMergeService
	logger *slog.Logger
}

// NewPlantMergeHandler creates a PlantMergeHandler.
func NewPlantMergeHandler(svc *service.PlantMergeService, logger *slog.Logger) *PlantMergeHandler {
	return &PlantMergeHandler{svc: svc, logger: logger}
}

// Preview handles POST /admin/plant-merges/preview (admin).
func (h *PlantMergeHandler) Preview(c *gin.Context) {
	var req dto.PlantMergeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(util.NewAppError(http.StatusBadRequest, constants.CodeBadRequest, constants.MsgInvalidParam))
		return
	}
	preview, err := h.svc.Preview(req.KeepPlantID, req.SourcePlantID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.OK(preview))
}

// Execute handles POST /admin/plant-merges (admin).
func (h *PlantMergeHandler) Execute(c *gin.Context) {
	var req dto.PlantMergeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(util.NewAppError(http.StatusBadRequest, constants.CodeBadRequest, constants.MsgInvalidParam))
		return
	}
	rec, err := h.svc.Execute(middleware.GetUserID(c), req.KeepPlantID, req.SourcePlantID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.OK(rec))
}

// List handles GET /admin/plant-merges (admin).
func (h *PlantMergeHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	items, total, err := h.svc.ListRecords(page, pageSize)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.OK(dto.PageData{List: items, Total: total, Page: page, Size: pageSize}))
}
