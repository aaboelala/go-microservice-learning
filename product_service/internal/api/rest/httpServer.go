package rest

import (
	"product_service/internal/api/rest/handler"
	"product_service/internal/help"

	"github.com/gofiber/fiber/v2"
)

func HandleRequests(app *fiber.App, userHandler *handler.UserHandler, helpHandler *help.HelpMethod) {
	auth := AuthMiddleware(helpHandler)

	// Private Endpoints (Protected by AuthMiddleware)
	private := app.Group("/products")
	private.Get("/my-products", auth, userHandler.ListMyProduct)
	private.Post("", auth, userHandler.CreateProduct)
	private.Put("/:id", auth, userHandler.UpdateProduct)
	private.Patch("/:id/status", auth, userHandler.PatchStatus)
	private.Patch("/:id/stock", auth, userHandler.PatchStock)
	private.Delete("/:id", auth, userHandler.DeleteProduct)

	// Public Endpoints (No Token Required)
	public := app.Group("/products")
	public.Get("", userHandler.ListProducts)
	public.Get("/:id", userHandler.GetProduct)
}

func AuthMiddleware(helpHandler *help.HelpMethod) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		authHeader := ctx.Get("Authorization")
		token, err := help.ExtractTokenFromHeader(authHeader)
		if err != nil {
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": err.Error(),
			})
		}

		claims, err := helpHandler.ValidateToken(token)
		if err != nil {
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"message": "Invalid or expired token",
			})
		}

		ctx.Locals("userID", claims.UserID)
		ctx.Locals("role", claims.Role)

		return ctx.Next()
	}
}
