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

// PlantMergeHandler exposes admin merge console endpoints.
type PlantMergeHandler struct {
	svc    *service.PlantMergeService
	logger *slog.Logger
}

// NewPlantMergeHandler creates a PlantMergeHandler.
func NewPlantMergeHandler(svc *service.PlantMergeService, logger *slog.Logger) *PlantMergeHandler {
	return &PlantMergeHandler{svc: svc, logger: logger}
}

// Preview handles POST /plants/merges/preview.
func (h *PlantMergeHandler) Preview(c *gin.Context) {
	var req dto.PlantMergeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(util.NewAppError(http.StatusBadRequest, constants.CodeBadRequest, constants.MsgInvalidParam+": "+err.Error()))
		return
	}
	preview, err := h.svc.Preview(req.TargetPlantID, req.SourcePlantIDs)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.OK(preview))
}

// Execute handles POST /plants/merges.
func (h *PlantMergeHandler) Execute(c *gin.Context) {
	var req dto.PlantMergeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(util.NewAppError(http.StatusBadRequest, constants.CodeBadRequest, constants.MsgInvalidParam+": "+err.Error()))
		return
	}
	result, stuck, err := h.svc.Execute(middleware.GetUserID(c), req.TargetPlantID, req.SourcePlantIDs)
	if err != nil {
		if stuck != nil {
			// 409 + structured payload: all records stayed in the pre-merge state;
			// the page marks the stuck source plant.
			c.JSON(http.StatusConflict, dto.Response{
				Code:    constants.CodeConflict,
				Message: "归并未执行：卡在「" + stuck.SourceName + "」(" + stuck.Step + ")，" + stuck.Reason,
				Data: gin.H{
					"stuck":            stuck,
					"target_plant_id":  req.TargetPlantID,
					"source_plant_ids": req.SourcePlantIDs,
					"rolled_back":      true,
				},
			})
			return
		}
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.OK(result))
}

// ListLogs handles GET /plants/merges.
func (h *PlantMergeHandler) ListLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	logs, total, err := h.svc.ListLogs(status, page, pageSize)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.OK(dto.PageData{List: logs, Total: total, Page: page, Size: pageSize}))
}

// ListMerged handles GET /plants/merged.
func (h *PlantMergeHandler) ListMerged(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("keyword")
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	plants, total, err := h.svc.ListMergedPlants(keyword, page, pageSize)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.OK(dto.PageData{List: plants, Total: total, Page: page, Size: pageSize}))
}
