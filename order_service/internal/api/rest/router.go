package rest

import (
	"order_service/internal/api/rest/handler"
	"order_service/internal/api/rest/middleware"
	"order_service/internal/helper"

	"github.com/gofiber/fiber/v2"
)

// SetupRoutes registers all REST API endpoints and applies middleware.
func SetupRoutes(app *fiber.App, orderHandler *handler.OrderHandler, tokenHelper *helper.TokenHelper) {
	auth := middleware.AuthMiddleware(tokenHelper)

	orders := app.Group("/orders", auth)
	orders.Post("", orderHandler.CreateOrder)
	orders.Get("", orderHandler.GetMyOrders)
	orders.Get("/:id", orderHandler.GetOrderDetails)
	orders.Post("/:id/cancel", orderHandler.CancelOrder)
}

