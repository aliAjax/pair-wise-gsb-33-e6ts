package router

import (
	"github.com/gin-gonic/gin"

	"github.com/gbplantwiki/gbplantwiki/internal/config"
	"github.com/gbplantwiki/gbplantwiki/internal/handler"
	"github.com/gbplantwiki/gbplantwiki/internal/middleware"
)

func registerPlantMergeRoutes(v1 *gin.RouterGroup, cfg *config.Config, h *handler.PlantMergeHandler, limiter *middleware.RateLimiter) {
	admin := v1.Group("/admin/plant-merges", middleware.AuthRequired(cfg), middleware.RequireRole("admin"))
	admin.GET("", h.List)
	admin.POST("/preview", limiter.Limit(), h.Preview)
	admin.POST("", limiter.Limit(), h.Execute)
}
