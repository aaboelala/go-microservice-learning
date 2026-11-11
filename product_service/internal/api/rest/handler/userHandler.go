package handler

import (
	"errors"
	"strconv"

	"product_service/internal/dto"
	"product_service/internal/help"
	"product_service/internal/service"

	"github.com/gofiber/fiber/v2"
)

type UserHandler struct {
	svc service.Service
}

func NewHandler(svc service.Service) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) ListProducts(ctx *fiber.Ctx) error {
	strpage := ctx.Query("page", "1")
	page, err := strconv.Atoi(strpage)
	if err != nil || page < 1 {
		page = 1
	}

	res, err := h.svc.ListProducts(ctx.Context(), page)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error occurred while fetching products",
		})
	}

	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":  "List of products",
		"products": res,
	})
}

func (h *UserHandler) CreateProduct(ctx *fiber.Ctx) error {
	var input dto.ProductCreateRequest
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	ownerID, err := help.GetUserID(ctx)
	if err != nil {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	err = h.svc.CreateProduct(ctx.Context(), ownerID, &input)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error occurred while creating product",
		})
	}

	return ctx.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "Product created",
	})
}

func (h *UserHandler) GetProduct(ctx *fiber.Ctx) error {
	inputID := ctx.Params("id")
	id, err := strconv.Atoi(inputID)
	if err != nil || id <= 0 {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid product ID",
		})
	}

	product, err := h.svc.GetProduct(ctx.Context(), uint(id))
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Product not found",
			})
		}
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error occurred while fetching product",
		})
	}

	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Get product",
		"product": product,
	})
}

func (h *UserHandler) ListMyProduct(ctx *fiber.Ctx) error {
	strpage := ctx.Query("page", "1")
	page, err := strconv.Atoi(strpage)
	if err != nil || page < 1 {
		page = 1
	}

	userID, err := help.GetUserID(ctx)
	if err != nil {
		return ctx.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"message": "Unauthorized",
		})
	}

	res, err := h.svc.ListMyProduct(ctx.Context(), userID, page)
	if err != nil {
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error occurred while fetching my products",
		})
	}

	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"message":  "List of my products",
		"products": res,
	})
}

func (h *UserHandler) UpdateProduct(ctx *fiber.Ctx) error {
	var input dto.ProductUpdateRequest
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	inputID := ctx.Params("id")
	id, err := strconv.Atoi(inputID)
	if err != nil || id <= 0 {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid product ID",
		})
	}

	err = h.svc.UpdateProduct(ctx.Context(), uint(id), &input)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Product not found",
			})
		}
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error occurred while updating product",
		})
	}

	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Product updated",
	})
}

func (h *UserHandler) PatchStatus(ctx *fiber.Ctx) error {
	var input dto.ProductPatchStatusRequest
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	inputID := ctx.Params("id")
	id, err := strconv.Atoi(inputID)
	if err != nil || id <= 0 {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid product ID",
		})
	}

	err = h.svc.PatchStatus(ctx.Context(), uint(id), input.Status)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Product not found",
			})
		}
		if errors.Is(err, service.ErrEmptyStatus) {
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": err.Error(),
			})
		}
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error occurred while updating product status",
		})
	}

	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Product status updated",
	})
}

func (h *UserHandler) PatchStock(ctx *fiber.Ctx) error {
	var input dto.ProductPatchStockRequest
	if err := ctx.BodyParser(&input); err != nil {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid request body",
		})
	}

	inputID := ctx.Params("id")
	id, err := strconv.Atoi(inputID)
	if err != nil || id <= 0 {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid product ID",
		})
	}

	err = h.svc.PatchStock(ctx.Context(), uint(id), input.Stock)
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Product not found",
			})
		}
		if errors.Is(err, service.ErrInvalidStock) {
			return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"message": err.Error(),
			})
		}
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error occurred while updating product stock",
		})
	}

	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Product stock updated",
	})
}

func (h *UserHandler) DeleteProduct(ctx *fiber.Ctx) error {
	inputID := ctx.Params("id")
	id, err := strconv.Atoi(inputID)
	if err != nil || id <= 0 {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid product ID",
		})
	}

	err = h.svc.DeleteProduct(ctx.Context(), uint(id))
	if err != nil {
		if errors.Is(err, service.ErrProductNotFound) {
			return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"message": "Product not found",
			})
		}
		return ctx.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"message": "Error occurred while deleting product",
		})
	}

	return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
		"message": "Product deleted",
	})
}
