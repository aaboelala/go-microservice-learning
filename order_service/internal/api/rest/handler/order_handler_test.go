package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"order_service/internal/api/rest"
	"order_service/internal/api/rest/handler"
	"order_service/internal/dto"
	"order_service/internal/helper"
	"order_service/internal/service"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "unit-test-secret-key-12345"

type mockOrderService struct {
	createOrderFn  func(ctx context.Context, userID uint, req *dto.CreateOrderRequest) (*dto.OrderResponse, error)
	getOrdersFn    func(ctx context.Context, userID uint, page, limit int) (*dto.PaginatedOrdersResponse, error)
	getOrderByIDFn func(ctx context.Context, userID uint, orderID uint64) (*dto.OrderResponse, error)
	cancelOrderFn  func(ctx context.Context, userID uint, orderID uint64) (*dto.OrderResponse, error)
}

func (m *mockOrderService) CreateOrder(ctx context.Context, userID uint, req *dto.CreateOrderRequest) (*dto.OrderResponse, error) {
	if m.createOrderFn != nil {
		return m.createOrderFn(ctx, userID, req)
	}
	return nil, errors.New("unimplemented")
}

func (m *mockOrderService) GetOrders(ctx context.Context, userID uint, page, limit int) (*dto.PaginatedOrdersResponse, error) {
	if m.getOrdersFn != nil {
		return m.getOrdersFn(ctx, userID, page, limit)
	}
	return nil, errors.New("unimplemented")
}

func (m *mockOrderService) GetOrderByID(ctx context.Context, userID uint, orderID uint64) (*dto.OrderResponse, error) {
	if m.getOrderByIDFn != nil {
		return m.getOrderByIDFn(ctx, userID, orderID)
	}
	return nil, service.ErrOrderNotFound
}

func (m *mockOrderService) CancelOrder(ctx context.Context, userID uint, orderID uint64) (*dto.OrderResponse, error) {
	if m.cancelOrderFn != nil {
		return m.cancelOrderFn(ctx, userID, orderID)
	}
	return nil, service.ErrOrderNotFound
}

func setupTestApp(svc service.OrderService) *fiber.App {
	app := fiber.New()
	orderHandler := handler.NewOrderHandler(svc)
	tokenHelper := helper.NewTokenHelper(testSecret)
	rest.SetupRoutes(app, orderHandler, tokenHelper)
	return app
}

func createValidToken(userID uint) string {
	claims := &helper.UserClaims{
		UserID: userID,
		Role:   "customer",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString([]byte(testSecret))
	return tokenStr
}

func TestHandler_CreateOrder_AuthRequired(t *testing.T) {
	app := setupTestApp(&mockOrderService{})

	// Missing token
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(`{"items":[{"product_id":1,"quantity":1}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// Invalid token
	reqInvalid := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(`{"items":[{"product_id":1,"quantity":1}]}`))
	reqInvalid.Header.Set("Content-Type", "application/json")
	reqInvalid.Header.Set("Authorization", "Bearer invalid-token-string")
	respInvalid, err := app.Test(reqInvalid)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, respInvalid.StatusCode)
}

func TestHandler_CreateOrder_Success(t *testing.T) {
	mockSvc := &mockOrderService{
		createOrderFn: func(ctx context.Context, userID uint, req *dto.CreateOrderRequest) (*dto.OrderResponse, error) {
			assert.Equal(t, uint(10), userID)
			assert.Len(t, req.Items, 1)
			return &dto.OrderResponse{
				ID:        100,
				UserID:    userID,
				Status:    "PENDING",
				Total:     49.99,
				CreatedAt: time.Now(),
				Items: []dto.OrderItemResponse{
					{ID: 1, OrderID: 100, ProductID: 1, ProductName: "Keyboard", Quantity: 1, UnitPrice: 49.99, LineTotal: 49.99},
				},
			}, nil
		},
	}

	app := setupTestApp(mockSvc)
	token := createValidToken(10)

	payload := `{"items":[{"product_id":1,"quantity":1}]}`
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var resBody map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&resBody)
	require.NoError(t, err)
	assert.True(t, resBody["success"].(bool))
	data := resBody["data"].(map[string]interface{})
	assert.Equal(t, float64(100), data["id"])
	assert.Equal(t, "PENDING", data["status"])
}

func TestHandler_CreateOrder_ErrorMappings(t *testing.T) {
	tests := []struct {
		name           string
		payload        string
		mockErr        error
		expectedStatus int
	}{
		{
			name:           "Empty Items Validation Error",
			payload:        `{"items":[]}`,
			mockErr:        dto.ErrEmptyOrderItems,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Duplicate Product ID",
			payload:        `{"items":[{"product_id":1,"quantity":1},{"product_id":1,"quantity":2}]}`,
			mockErr:        dto.ErrDuplicateProductItem,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Product Not Found",
			payload:        `{"items":[{"product_id":99,"quantity":1}]}`,
			mockErr:        service.ErrProductNotFound,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "Product Inactive",
			payload:        `{"items":[{"product_id":5,"quantity":1}]}`,
			mockErr:        service.ErrProductInactive,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "Insufficient Stock",
			payload:        `{"items":[{"product_id":3,"quantity":100}]}`,
			mockErr:        service.ErrInsufficientStock,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "Product Service Unavailable",
			payload:        `{"items":[{"product_id":1,"quantity":1}]}`,
			mockErr:        service.ErrProductServiceUnavailable,
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:           "Product Service Timeout",
			payload:        `{"items":[{"product_id":1,"quantity":1}]}`,
			mockErr:        service.ErrProductServiceTimeout,
			expectedStatus: http.StatusGatewayTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := &mockOrderService{
				createOrderFn: func(ctx context.Context, userID uint, req *dto.CreateOrderRequest) (*dto.OrderResponse, error) {
					return nil, tt.mockErr
				},
			}
			app := setupTestApp(mockSvc)
			token := createValidToken(1)

			req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(tt.payload))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

func TestHandler_GetMyOrders(t *testing.T) {
	mockSvc := &mockOrderService{
		getOrdersFn: func(ctx context.Context, userID uint, page, limit int) (*dto.PaginatedOrdersResponse, error) {
			assert.Equal(t, uint(5), userID)
			assert.Equal(t, 2, page)
			assert.Equal(t, 5, limit)
			return &dto.PaginatedOrdersResponse{
				Data: []dto.OrderResponse{
					{ID: 1, UserID: userID, Status: "PENDING", Total: 20.0},
				},
				Pagination: dto.PaginationMetadata{
					Page:       2,
					Limit:      5,
					TotalItems: 6,
					TotalPages: 2,
				},
			}, nil
		},
	}

	app := setupTestApp(mockSvc)
	token := createValidToken(5)

	req := httptest.NewRequest(http.MethodGet, "/orders?page=2&limit=5", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var resBody map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&resBody)
	require.NoError(t, err)
	assert.True(t, resBody["success"].(bool))
	data := resBody["data"].([]interface{})
	assert.Len(t, data, 1)
}

func TestHandler_GetOrderDetails(t *testing.T) {
	mockSvc := &mockOrderService{
		getOrderByIDFn: func(ctx context.Context, userID uint, orderID uint64) (*dto.OrderResponse, error) {
			if orderID == 404 {
				return nil, service.ErrOrderNotFound
			}
			return &dto.OrderResponse{
				ID:     orderID,
				UserID: userID,
				Status: "PENDING",
				Total:  30.0,
			}, nil
		},
	}

	app := setupTestApp(mockSvc)
	token := createValidToken(1)

	// Valid order
	reqSuccess := httptest.NewRequest(http.MethodGet, "/orders/10", nil)
	reqSuccess.Header.Set("Authorization", "Bearer "+token)
	respSuccess, err := app.Test(reqSuccess)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respSuccess.StatusCode)

	// Not found
	reqNF := httptest.NewRequest(http.MethodGet, "/orders/404", nil)
	reqNF.Header.Set("Authorization", "Bearer "+token)
	respNF, err := app.Test(reqNF)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, respNF.StatusCode)

	// Invalid ID format
	reqBad := httptest.NewRequest(http.MethodGet, "/orders/abc", nil)
	reqBad.Header.Set("Authorization", "Bearer "+token)
	respBad, err := app.Test(reqBad)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, respBad.StatusCode)
}

func TestHandler_CancelOrder(t *testing.T) {
	mockSvc := &mockOrderService{
		cancelOrderFn: func(ctx context.Context, userID uint, orderID uint64) (*dto.OrderResponse, error) {
			switch orderID {
			case 10:
				return &dto.OrderResponse{ID: 10, UserID: userID, Status: "CANCELLED"}, nil
			case 20:
				return nil, service.ErrOrderAlreadyCancelled
			case 30:
				return nil, service.ErrInvalidCancellationStatus
			default:
				return nil, service.ErrOrderNotFound
			}
		},
	}

	app := setupTestApp(mockSvc)
	token := createValidToken(1)

	// 1. Success cancel
	reqSuccess := httptest.NewRequest(http.MethodPost, "/orders/10/cancel", nil)
	reqSuccess.Header.Set("Authorization", "Bearer "+token)
	respSuccess, err := app.Test(reqSuccess)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respSuccess.StatusCode)

	// 2. Already cancelled
	reqAlready := httptest.NewRequest(http.MethodPost, "/orders/20/cancel", nil)
	reqAlready.Header.Set("Authorization", "Bearer "+token)
	respAlready, err := app.Test(reqAlready)
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, respAlready.StatusCode)

	// 3. Invalid cancellation status
	reqInvalid := httptest.NewRequest(http.MethodPost, "/orders/30/cancel", nil)
	reqInvalid.Header.Set("Authorization", "Bearer "+token)
	respInvalid, err := app.Test(reqInvalid)
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, respInvalid.StatusCode)

	// 4. Not found
	reqNF := httptest.NewRequest(http.MethodPost, "/orders/999/cancel", nil)
	reqNF.Header.Set("Authorization", "Bearer "+token)
	respNF, err := app.Test(reqNF)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, respNF.StatusCode)
}

