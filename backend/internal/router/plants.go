package router

import (
	"github.com/gin-gonic/gin"

	"github.com/gbplantwiki/gbplantwiki/internal/config"
	"github.com/gbplantwiki/gbplantwiki/internal/handler"
	"github.com/gbplantwiki/gbplantwiki/internal/middleware"
)

func registerPlantRoutes(v1 *gin.RouterGroup, cfg *config.Config, h *handler.PlantSpeciesHandler, mergeH *handler.PlantMergeHandler, limiter *middleware.RateLimiter) {
	plants := v1.Group("/plants")
	plants.GET("", h.List)
	plants.GET("/:id", h.Get)

	// Admin merge console (registered before the generic admin /:id group).
	adminMerge := plants.Group("", middleware.AuthRequired(cfg), middleware.RequireRole("admin"))
	adminMerge.GET("/merges", mergeH.ListLogs)
	adminMerge.GET("/merges/merged", mergeH.ListMerged)
	adminMerge.POST("/merges/preview", mergeH.Preview)
	adminMerge.POST("/merges", limiter.Limit(), mergeH.Execute)

	admin := plants.Group("", middleware.AuthRequired(cfg), middleware.RequireRole("admin"))
	admin.POST("", limiter.Limit(), h.Create)
	admin.PUT("/:id", h.Update)
	admin.DELETE("/:id", h.Delete)
}
