package middleware

import (
	"order_service/internal/helper"

	"github.com/gofiber/fiber/v2"
)

// AuthMiddleware creates a Fiber middleware that validates the JWT Bearer token
// and attaches the authenticated userID and role to the Fiber context locals.
func AuthMiddleware(tokenHelper *helper.TokenHelper) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		authHeader := ctx.Get("Authorization")
		token, err := helper.ExtractTokenFromHeader(authHeader)
		if err != nil {
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": err.Error(),
			})
		}

		claims, err := tokenHelper.ValidateToken(token)
		if err != nil {
			return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"success": false,
				"message": "Invalid or expired token",
			})
		}

		ctx.Locals("userID", claims.UserID)
		ctx.Locals("role", claims.Role)

		return ctx.Next()
	}
}

