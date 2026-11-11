package handler

import (
	"errors"
	"strconv"

	"github.com/ahmed/go-service/internal/dto"
	"github.com/ahmed/go-service/internal/repository"
	"github.com/ahmed/go-service/internal/service"
	"github.com/gofiber/fiber/v2"
)

// UserHandler defines HTTP endpoints for user resource.
type UserHandler struct {
	svc service.UserService
}

// NewUserHandler constructs a handler instance.
func NewUserHandler(svc service.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// Register handles POST /api/v1/users/register
func (h *UserHandler) Register(c *fiber.Ctx) error {
	var input dto.RegisterInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if input.Email == "" || input.Password == "" || input.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email, password, and name are required"})
	}

	res, err := h.svc.Register(c.UserContext(), input)
	if err != nil {
		if errors.Is(err, service.ErrUserAlreadyExists) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create user"})
	}

	return c.Status(fiber.StatusCreated).JSON(res)
}

// Login handles POST /api/v1/users/login
func (h *UserHandler) Login(c *fiber.Ctx) error {
	var input dto.LoginInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	res, err := h.svc.Login(c.UserContext(), input)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Login failed"})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

// GetProfile handles GET /api/v1/users/me (Authenticated)
func (h *UserHandler) GetProfile(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	res, err := h.svc.GetByID(c.UserContext(), userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch user"})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

// GetByID handles GET /api/v1/users/:id
func (h *UserHandler) GetByID(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	res, err := h.svc.GetByID(c.UserContext(), uint(id))
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch user"})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

// Update handles PUT /api/v1/users/:id
func (h *UserHandler) Update(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	currentUserID, _ := c.Locals("userID").(uint)
	userRole, _ := c.Locals("userRole").(string)

	if currentUserID != uint(id) && userRole != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You can only update your own account"})
	}

	var input dto.UpdateUserInput
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	res, err := h.svc.Update(c.UserContext(), uint(id), input)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update user"})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

// Delete handles DELETE /api/v1/users/:id
func (h *UserHandler) Delete(c *fiber.Ctx) error {
	idParam := c.Params("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}

	currentUserID, _ := c.Locals("userID").(uint)
	userRole, _ := c.Locals("userRole").(string)

	if currentUserID != uint(id) && userRole != "admin" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You can only delete your own account"})
	}

	if err := h.svc.Delete(c.UserContext(), uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete user"})
	}

	return c.SendStatus(fiber.StatusNoContent)
}
