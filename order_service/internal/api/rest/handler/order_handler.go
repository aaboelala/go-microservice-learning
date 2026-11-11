package handler

import (
	"errors"
	"log"
	"strconv"

	"order_service/internal/dto"
	"order_service/internal/service"

	"github.com/gofiber/fiber/v2"
)

// OrderHandler handles HTTP requests for order management.
type OrderHandler struct {
	orderService service.OrderService
}

// NewOrderHandler creates a new OrderHandler with the injected OrderService.
func NewOrderHandler(orderService service.OrderService) *OrderHandler {
	return &OrderHandler{orderService: orderService}
}

// CreateOrder handles POST /orders
func (h *OrderHandler) CreateOrder(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(uint)
	if !ok || userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Unauthorized user",
		})
	}

	var req dto.CreateOrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid request body",
		})
	}

	order, err := h.orderService.CreateOrder(c.UserContext(), userID, &req)
	if err != nil {
		return h.handleServiceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success": true,
		"data":    order,
	})
}

// GetMyOrders handles GET /orders?page=1&limit=10
func (h *OrderHandler) GetMyOrders(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(uint)
	if !ok || userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Unauthorized user",
		})
	}

	page := c.QueryInt("page", 1)
	limit := c.QueryInt("limit", 10)

	resp, err := h.orderService.GetOrders(c.UserContext(), userID, page, limit)
	if err != nil {
		return h.handleServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success":    true,
		"data":       resp.Data,
		"pagination": resp.Pagination,
	})
}

// GetOrderDetails handles GET /orders/:id
func (h *OrderHandler) GetOrderDetails(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(uint)
	if !ok || userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Unauthorized user",
		})
	}

	orderIDParam := c.Params("id")
	orderID, err := strconv.ParseUint(orderIDParam, 10, 64)
	if err != nil || orderID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid order id",
		})
	}

	order, err := h.orderService.GetOrderByID(c.UserContext(), userID, orderID)
	if err != nil {
		return h.handleServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"data":    order,
	})
}

// CancelOrder handles POST /orders/:id/cancel
func (h *OrderHandler) CancelOrder(c *fiber.Ctx) error {
	userID, ok := c.Locals("userID").(uint)
	if !ok || userID == 0 {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"success": false,
			"message": "Unauthorized user",
		})
	}

	orderIDParam := c.Params("id")
	orderID, err := strconv.ParseUint(orderIDParam, 10, 64)
	if err != nil || orderID == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": "Invalid order id",
		})
	}

	order, err := h.orderService.CancelOrder(c.UserContext(), userID, orderID)
	if err != nil {
		return h.handleServiceError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"success": true,
		"message": "Order cancelled successfully",
		"data":    order,
	})
}

func (h *OrderHandler) handleServiceError(c *fiber.Ctx, err error) error {
	// Request validation errors -> 400 Bad Request
	if errors.Is(err, dto.ErrEmptyOrderItems) ||
		errors.Is(err, dto.ErrInvalidItemQuantity) ||
		errors.Is(err, dto.ErrInvalidProductID) ||
		errors.Is(err, dto.ErrDuplicateProductItem) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"message": err.Error(),
		})
	}

	// Product lookup issues -> 422 Unprocessable Entity
	if errors.Is(err, service.ErrProductNotFound) || errors.Is(err, service.ErrProductInactive) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"success": false,
			"message": err.Error(),
		})
	}

	// Stock insufficiency or invalid state transition -> 409 Conflict
	if errors.Is(err, service.ErrInsufficientStock) ||
		errors.Is(err, service.ErrOrderAlreadyCancelled) ||
		errors.Is(err, service.ErrInvalidCancellationStatus) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"success": false,
			"message": err.Error(),
		})
	}

	// Order not found (or cross-user access) -> 404 Not Found
	if errors.Is(err, service.ErrOrderNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"success": false,
			"message": "Order not found",
		})
	}

	// External Product Service gRPC errors
	if errors.Is(err, service.ErrProductServiceUnavailable) {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"success": false,
			"message": "Product service is temporarily unavailable",
		})
	}

	if errors.Is(err, service.ErrProductServiceTimeout) {
		return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{
			"success": false,
			"message": "Product service request timed out",
		})
	}

	// Default fallback for unexpected database or internal errors -> 500
	log.Printf("[OrderHandler] Internal error: %v", err)
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"success": false,
		"message": "Internal server error",
	})
}

