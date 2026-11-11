package rest

import (
	"strings"

	"github.com/ahmed/go-service/internal/api/rest/handler"
	"github.com/ahmed/go-service/internal/helper"
	"github.com/gofiber/fiber/v2"
)

// SetupRoutes registers middleware and maps endpoint routes.
func SetupRoutes(app *fiber.App, userHandler *handler.UserHandler, h *helper.Helper) {
	api := app.Group("/api/v1")

	// Public routes
	users := api.Group("/users")
	users.Post("/register", userHandler.Register)
	users.Post("/login", userHandler.Login)

	// Protected routes (Requires AuthMiddleware)
	protected := users.Group("", AuthMiddleware(h))
	protected.Get("/me", userHandler.GetProfile)
	protected.Get("/:id", userHandler.GetByID)
	protected.Put("/:id", userHandler.Update)
	protected.Delete("/:id", userHandler.Delete)
}

// AuthMiddleware creates a Fiber handler that validates JWT Bearer tokens.
//
// WHY: Protects endpoints against unauthenticated access.
// HOW: Extracts 'Authorization: Bearer <token>', verifies signature with helper, stores claims in context.
func AuthMiddleware(h *helper.Helper) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing Authorization header"})
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid Authorization header format format: Bearer <token>"})
		}

		claims, err := h.ValidateToken(parts[1])
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired token"})
		}

		// Attach user identity to request context for handlers to consume
		c.Locals("userID", claims.UserID)
		c.Locals("userRole", claims.Role)

		return c.Next()
	}
}
