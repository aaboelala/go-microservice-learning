package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"order_service/internal/client"
	"order_service/internal/dto"
	"order_service/internal/entity"
	"order_service/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Product Client ---

type mockProductClient struct {
	getProductFn func(ctx context.Context, id uint64) (*client.ProductInfo, error)
	closeFn      func() error
}

func (m *mockProductClient) GetProduct(ctx context.Context, id uint64) (*client.ProductInfo, error) {
	if m.getProductFn != nil {
		return m.getProductFn(ctx, id)
	}
	return nil, client.ErrProductNotFound
}

func (m *mockProductClient) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

// --- Mock Order Repository ---

type mockOrderRepository struct {
	createOrderWithItemsFn func(ctx context.Context, order *entity.Order) error
	getOrdersByUserIDFn    func(ctx context.Context, userID uint, offset, limit int) ([]entity.Order, int64, error)
	getOrderByIDFn         func(ctx context.Context, orderID uint64) (*entity.Order, error)
	updateOrderStatusFn    func(ctx context.Context, orderID uint64, oldStatus, newStatus entity.OrderStatus) error
}

func (m *mockOrderRepository) CreateOrderWithItems(ctx context.Context, order *entity.Order) error {
	if m.createOrderWithItemsFn != nil {
		return m.createOrderWithItemsFn(ctx, order)
	}
	return nil
}

func (m *mockOrderRepository) GetOrdersByUserID(ctx context.Context, userID uint, offset, limit int) ([]entity.Order, int64, error) {
	if m.getOrdersByUserIDFn != nil {
		return m.getOrdersByUserIDFn(ctx, userID, offset, limit)
	}
	return nil, 0, nil
}

func (m *mockOrderRepository) GetOrderByID(ctx context.Context, orderID uint64) (*entity.Order, error) {
	if m.getOrderByIDFn != nil {
		return m.getOrderByIDFn(ctx, orderID)
	}
	return nil, service.ErrOrderNotFound
}

func (m *mockOrderRepository) UpdateOrderStatus(ctx context.Context, orderID uint64, oldStatus, newStatus entity.OrderStatus) error {
	if m.updateOrderStatusFn != nil {
		return m.updateOrderStatusFn(ctx, orderID, oldStatus, newStatus)
	}
	return nil
}

// =========================================================================
// Tests: Create Order
// =========================================================================

func TestCreateOrder_Success(t *testing.T) {
	ctx := context.Background()
	userID := uint(42)

	mockProd := &mockProductClient{
		getProductFn: func(ctx context.Context, id uint64) (*client.ProductInfo, error) {
			switch id {
			case 1:
				return &client.ProductInfo{
					ID:       1,
					Name:     "Mechanical Keyboard",
					Price:    99.99,
					Stock:    10,
					Status:   "ACTIVE",
					IsActive: true,
				}, nil
			case 2:
				return &client.ProductInfo{
					ID:       2,
					Name:     "Mousepad",
					Price:    15.50,
					Stock:    5,
					Status:   "active",
					IsActive: true,
				}, nil
			default:
				return nil, client.ErrProductNotFound
			}
		},
	}

	var savedOrder *entity.Order
	mockRepo := &mockOrderRepository{
		createOrderWithItemsFn: func(ctx context.Context, order *entity.Order) error {
			order.ID = 1001
			for i := range order.Items {
				order.Items[i].ID = uint64(2001 + i)
				order.Items[i].OrderID = order.ID
			}
			savedOrder = order
			return nil
		},
	}

	svc := service.NewOrderService(mockRepo, mockProd)

	req := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{
			{ProductID: 1, Quantity: 2}, // 2 * 99.99 = 199.98
			{ProductID: 2, Quantity: 1}, // 1 * 15.50 = 15.50
		},
	}

	resp, err := svc.CreateOrder(ctx, userID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Equal(t, uint64(1001), resp.ID)
	assert.Equal(t, userID, resp.UserID)
	assert.Equal(t, string(entity.OrderStatusPending), resp.Status)
	// Expected total: 199.98 + 15.50 = 215.48
	assert.InDelta(t, 215.48, resp.Total, 0.001)
	assert.Len(t, resp.Items, 2)

	assert.Equal(t, "Mechanical Keyboard", resp.Items[0].ProductName)
	assert.Equal(t, 2, resp.Items[0].Quantity)
	assert.InDelta(t, 99.99, resp.Items[0].UnitPrice, 0.001)
	assert.InDelta(t, 199.98, resp.Items[0].LineTotal, 0.001)

	assert.Equal(t, "Mousepad", resp.Items[1].ProductName)
	assert.Equal(t, 1, resp.Items[1].Quantity)
	assert.InDelta(t, 15.50, resp.Items[1].UnitPrice, 0.001)
	assert.InDelta(t, 15.50, resp.Items[1].LineTotal, 0.001)

	require.NotNil(t, savedOrder)
	assert.Equal(t, uint64(1001), savedOrder.ID)
}

func TestCreateOrder_EmptyItems(t *testing.T) {
	svc := service.NewOrderService(&mockOrderRepository{}, &mockProductClient{})

	req := &dto.CreateOrderRequest{Items: []dto.CreateOrderItemRequest{}}
	_, err := svc.CreateOrder(context.Background(), 1, req)
	assert.ErrorIs(t, err, dto.ErrEmptyOrderItems)

	_, errNil := svc.CreateOrder(context.Background(), 1, nil)
	assert.ErrorIs(t, errNil, dto.ErrEmptyOrderItems)
}

func TestCreateOrder_ZeroOrNegativeQuantity(t *testing.T) {
	svc := service.NewOrderService(&mockOrderRepository{}, &mockProductClient{})

	// Zero quantity
	reqZero := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{{ProductID: 1, Quantity: 0}},
	}
	_, err := svc.CreateOrder(context.Background(), 1, reqZero)
	assert.ErrorIs(t, err, dto.ErrInvalidItemQuantity)

	// Negative quantity
	reqNeg := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{{ProductID: 1, Quantity: -3}},
	}
	_, errNeg := svc.CreateOrder(context.Background(), 1, reqNeg)
	assert.ErrorIs(t, errNeg, dto.ErrInvalidItemQuantity)
}

func TestCreateOrder_DuplicateProductIDs(t *testing.T) {
	svc := service.NewOrderService(&mockOrderRepository{}, &mockProductClient{})

	req := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{
			{ProductID: 1, Quantity: 2},
			{ProductID: 2, Quantity: 1},
			{ProductID: 1, Quantity: 3}, // duplicate of product 1
		},
	}
	_, err := svc.CreateOrder(context.Background(), 1, req)
	assert.ErrorIs(t, err, dto.ErrDuplicateProductItem)
}

func TestCreateOrder_ProductNotFound(t *testing.T) {
	mockProd := &mockProductClient{
		getProductFn: func(ctx context.Context, id uint64) (*client.ProductInfo, error) {
			return nil, client.ErrProductNotFound
		},
	}
	svc := service.NewOrderService(&mockOrderRepository{}, mockProd)

	req := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{{ProductID: 999, Quantity: 1}},
	}
	_, err := svc.CreateOrder(context.Background(), 1, req)
	assert.ErrorIs(t, err, service.ErrProductNotFound)
}

func TestCreateOrder_InactiveProduct(t *testing.T) {
	mockProd := &mockProductClient{
		getProductFn: func(ctx context.Context, id uint64) (*client.ProductInfo, error) {
			return &client.ProductInfo{
				ID:       id,
				Name:     "Discontinued Gadget",
				Price:    50.0,
				Stock:    10,
				Status:   "INACTIVE",
				IsActive: false,
			}, nil
		},
	}
	svc := service.NewOrderService(&mockOrderRepository{}, mockProd)

	req := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{{ProductID: 10, Quantity: 1}},
	}
	_, err := svc.CreateOrder(context.Background(), 1, req)
	assert.ErrorIs(t, err, service.ErrProductInactive)
}

func TestCreateOrder_InsufficientStock(t *testing.T) {
	mockProd := &mockProductClient{
		getProductFn: func(ctx context.Context, id uint64) (*client.ProductInfo, error) {
			return &client.ProductInfo{
				ID:       id,
				Name:     "Limited Edition Watch",
				Price:    200.0,
				Stock:    2, // only 2 in stock
				Status:   "ACTIVE",
				IsActive: true,
			}, nil
		},
	}
	svc := service.NewOrderService(&mockOrderRepository{}, mockProd)

	req := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{{ProductID: 15, Quantity: 5}}, // requests 5
	}
	_, err := svc.CreateOrder(context.Background(), 1, req)
	assert.ErrorIs(t, err, service.ErrInsufficientStock)
}

func TestCreateOrder_ProductServiceUnavailable(t *testing.T) {
	mockProd := &mockProductClient{
		getProductFn: func(ctx context.Context, id uint64) (*client.ProductInfo, error) {
			return nil, client.ErrProductServiceUnavailable
		},
	}
	svc := service.NewOrderService(&mockOrderRepository{}, mockProd)

	req := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{{ProductID: 1, Quantity: 1}},
	}
	_, err := svc.CreateOrder(context.Background(), 1, req)
	assert.ErrorIs(t, err, service.ErrProductServiceUnavailable)
}

func TestCreateOrder_ProductServiceTimeout(t *testing.T) {
	mockProd := &mockProductClient{
		getProductFn: func(ctx context.Context, id uint64) (*client.ProductInfo, error) {
			return nil, client.ErrProductServiceTimeout
		},
	}
	svc := service.NewOrderService(&mockOrderRepository{}, mockProd)

	req := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{{ProductID: 1, Quantity: 1}},
	}
	_, err := svc.CreateOrder(context.Background(), 1, req)
	assert.ErrorIs(t, err, service.ErrProductServiceTimeout)
}

func TestCreateOrder_DatabaseRollbackOnSaveFailure(t *testing.T) {
	mockProd := &mockProductClient{
		getProductFn: func(ctx context.Context, id uint64) (*client.ProductInfo, error) {
			return &client.ProductInfo{
				ID:       id,
				Name:     "Item",
				Price:    10.0,
				Stock:    100,
				Status:   "ACTIVE",
				IsActive: true,
			}, nil
		},
	}

	dbErr := errors.New("database connection lost during transaction")
	mockRepo := &mockOrderRepository{
		createOrderWithItemsFn: func(ctx context.Context, order *entity.Order) error {
			return dbErr
		},
	}

	svc := service.NewOrderService(mockRepo, mockProd)

	req := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{{ProductID: 1, Quantity: 1}},
	}
	_, err := svc.CreateOrder(context.Background(), 1, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create order")
}

func TestCreateOrder_FinancialPrecision(t *testing.T) {
	// Floating point edge cases: e.g. 0.1 + 0.2 != 0.3 in IEEE 754 float
	// Price: 19.99 * 3 = 59.97
	// Price: 3.33 * 3 = 9.99
	mockProd := &mockProductClient{
		getProductFn: func(ctx context.Context, id uint64) (*client.ProductInfo, error) {
			switch id {
			case 1:
				return &client.ProductInfo{
					ID: 1, Name: "Item 1", Price: 19.99, Stock: 10, Status: "ACTIVE", IsActive: true,
				}, nil
			case 2:
				return &client.ProductInfo{
					ID: 2, Name: "Item 2", Price: 3.33, Stock: 10, Status: "ACTIVE", IsActive: true,
				}, nil
			}
			return nil, client.ErrProductNotFound
		},
	}

	mockRepo := &mockOrderRepository{
		createOrderWithItemsFn: func(ctx context.Context, order *entity.Order) error {
			return nil
		},
	}

	svc := service.NewOrderService(mockRepo, mockProd)
	req := &dto.CreateOrderRequest{
		Items: []dto.CreateOrderItemRequest{
			{ProductID: 1, Quantity: 3}, // 59.97
			{ProductID: 2, Quantity: 3}, // 9.99
		},
	}

	resp, err := svc.CreateOrder(context.Background(), 1, req)
	require.NoError(t, err)
	assert.Equal(t, 69.96, resp.Total)
	assert.Equal(t, 59.97, resp.Items[0].LineTotal)
	assert.Equal(t, 9.99, resp.Items[1].LineTotal)
}

// =========================================================================
// Tests: Get Orders (Pagination & Isolation)
// =========================================================================

func TestGetOrders_UserScopingAndPagination(t *testing.T) {
	targetUserID := uint(5)

	orders := []entity.Order{
		{
			ID:        2,
			UserID:    targetUserID,
			Status:    entity.OrderStatusPending,
			Total:     100.0,
			CreatedAt: time.Now(),
		},
		{
			ID:        1,
			UserID:    targetUserID,
			Status:    entity.OrderStatusCompleted,
			Total:     50.0,
			CreatedAt: time.Now().Add(-1 * time.Hour),
		},
	}

	mockRepo := &mockOrderRepository{
		getOrdersByUserIDFn: func(ctx context.Context, userID uint, offset, limit int) ([]entity.Order, int64, error) {
			assert.Equal(t, targetUserID, userID)
			assert.Equal(t, 0, offset)
			assert.Equal(t, 10, limit)
			return orders, 2, nil
		},
	}

	svc := service.NewOrderService(mockRepo, &mockProductClient{})

	resp, err := svc.GetOrders(context.Background(), targetUserID, 1, 10)
	require.NoError(t, err)
	require.NotNil(t, resp)

	assert.Len(t, resp.Data, 2)
	assert.Equal(t, int64(2), resp.Pagination.TotalItems)
	assert.Equal(t, 1, resp.Pagination.TotalPages)
	assert.Equal(t, 1, resp.Pagination.Page)
	assert.Equal(t, 10, resp.Pagination.Limit)
}

// =========================================================================
// Tests: Get Order Details
// =========================================================================

func TestGetOrderByID_Success(t *testing.T) {
	order := &entity.Order{
		ID:        50,
		UserID:    10,
		Status:    entity.OrderStatusPending,
		Total:     85.0,
		CreatedAt: time.Now(),
		Items: []entity.OrderItem{
			{ID: 1, OrderID: 50, ProductID: 1, ProductNameSnapshot: "Keyboard", Quantity: 1, UnitPriceSnapshot: 85.0, LineTotal: 85.0},
		},
	}

	mockRepo := &mockOrderRepository{
		getOrderByIDFn: func(ctx context.Context, orderID uint64) (*entity.Order, error) {
			assert.Equal(t, uint64(50), orderID)
			return order, nil
		},
	}

	svc := service.NewOrderService(mockRepo, &mockProductClient{})

	resp, err := svc.GetOrderByID(context.Background(), 10, 50)
	require.NoError(t, err)
	assert.Equal(t, uint64(50), resp.ID)
	assert.Equal(t, uint(10), resp.UserID)
	assert.Len(t, resp.Items, 1)
}

func TestGetOrderByID_CrossUserAccessForbidden(t *testing.T) {
	order := &entity.Order{
		ID:     50,
		UserID: 99, // Owned by user 99
	}

	mockRepo := &mockOrderRepository{
		getOrderByIDFn: func(ctx context.Context, orderID uint64) (*entity.Order, error) {
			return order, nil
		},
	}

	svc := service.NewOrderService(mockRepo, &mockProductClient{})

	// User 10 tries to access user 99's order
	_, err := svc.GetOrderByID(context.Background(), 10, 50)
	assert.ErrorIs(t, err, service.ErrOrderNotFound)
}

func TestGetOrderByID_NotFound(t *testing.T) {
	mockRepo := &mockOrderRepository{
		getOrderByIDFn: func(ctx context.Context, orderID uint64) (*entity.Order, error) {
			return nil, service.ErrOrderNotFound
		},
	}

	svc := service.NewOrderService(mockRepo, &mockProductClient{})

	_, err := svc.GetOrderByID(context.Background(), 1, 9999)
	assert.ErrorIs(t, err, service.ErrOrderNotFound)
}

// =========================================================================
// Tests: Cancel Order
// =========================================================================

func TestCancelOrder_Success(t *testing.T) {
	order := &entity.Order{
		ID:     12,
		UserID: 7,
		Status: entity.OrderStatusPending,
	}

	var statusUpdated bool
	mockRepo := &mockOrderRepository{
		getOrderByIDFn: func(ctx context.Context, orderID uint64) (*entity.Order, error) {
			return order, nil
		},
		updateOrderStatusFn: func(ctx context.Context, orderID uint64, oldStatus, newStatus entity.OrderStatus) error {
			assert.Equal(t, uint64(12), orderID)
			assert.Equal(t, entity.OrderStatusPending, oldStatus)
			assert.Equal(t, entity.OrderStatusCancelled, newStatus)
			statusUpdated = true
			return nil
		},
	}

	svc := service.NewOrderService(mockRepo, &mockProductClient{})

	resp, err := svc.CancelOrder(context.Background(), 7, 12)
	require.NoError(t, err)
	assert.True(t, statusUpdated)
	assert.Equal(t, string(entity.OrderStatusCancelled), resp.Status)
}

func TestCancelOrder_BelongsToAnotherUser(t *testing.T) {
	order := &entity.Order{
		ID:     12,
		UserID: 7, // Owned by user 7
		Status: entity.OrderStatusPending,
	}

	mockRepo := &mockOrderRepository{
		getOrderByIDFn: func(ctx context.Context, orderID uint64) (*entity.Order, error) {
			return order, nil
		},
	}

	svc := service.NewOrderService(mockRepo, &mockProductClient{})

	// User 8 attempts to cancel user 7's order
	_, err := svc.CancelOrder(context.Background(), 8, 12)
	assert.ErrorIs(t, err, service.ErrOrderNotFound)
}

func TestCancelOrder_AlreadyCancelled(t *testing.T) {
	order := &entity.Order{
		ID:     12,
		UserID: 7,
		Status: entity.OrderStatusCancelled,
	}

	mockRepo := &mockOrderRepository{
		getOrderByIDFn: func(ctx context.Context, orderID uint64) (*entity.Order, error) {
			return order, nil
		},
	}

	svc := service.NewOrderService(mockRepo, &mockProductClient{})

	_, err := svc.CancelOrder(context.Background(), 7, 12)
	assert.ErrorIs(t, err, service.ErrOrderAlreadyCancelled)
}

func TestCancelOrder_UnsupportedStatus(t *testing.T) {
	order := &entity.Order{
		ID:     12,
		UserID: 7,
		Status: entity.OrderStatusCompleted,
	}

	mockRepo := &mockOrderRepository{
		getOrderByIDFn: func(ctx context.Context, orderID uint64) (*entity.Order, error) {
			return order, nil
		},
	}

	svc := service.NewOrderService(mockRepo, &mockProductClient{})

	_, err := svc.CancelOrder(context.Background(), 7, 12)
	assert.ErrorIs(t, err, service.ErrInvalidCancellationStatus)
}

