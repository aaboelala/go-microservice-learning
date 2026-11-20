package repository_test

import (
	"context"
	"testing"
	"time"

	"order_service/internal/entity"
	"order_service/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&entity.Order{}, &entity.OrderItem{}, &entity.OutboxEvent{})
	require.NoError(t, err)

	return db
}

func TestOrderRepository_CreateOrderWithItems_Success(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewOrderRepository(db)

	order := &entity.Order{
		UserID: 10,
		Status: entity.OrderStatusPending,
		Total:  75.50,
		Items: []entity.OrderItem{
			{
				ProductID:           1,
				ProductNameSnapshot: "Product 1",
				Quantity:            2,
				UnitPriceSnapshot:   25.00,
				LineTotal:           50.00,
			},
			{
				ProductID:           2,
				ProductNameSnapshot: "Product 2",
				Quantity:            1,
				UnitPriceSnapshot:   25.50,
				LineTotal:           25.50,
			},
		},
	}

	err := repo.CreateOrderWithItems(context.Background(), order)
	require.NoError(t, err)
	assert.NotZero(t, order.ID)
	assert.Len(t, order.Items, 2)
	assert.Equal(t, order.ID, order.Items[0].OrderID)
	assert.Equal(t, order.ID, order.Items[1].OrderID)

	// Verify persistence in DB
	fetched, err := repo.GetOrderByID(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, uint(10), fetched.UserID)
	assert.Equal(t, entity.OrderStatusPending, fetched.Status)
	assert.Equal(t, 75.50, fetched.Total)
	assert.Len(t, fetched.Items, 2)
}

func TestOrderRepository_GetOrdersByUserID_Pagination(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewOrderRepository(db)

	now := time.Now()
	// Insert 5 orders for user 1, and 2 orders for user 2
	for i := 1; i <= 5; i++ {
		o := &entity.Order{
			UserID:    1,
			Status:    entity.OrderStatusPending,
			Total:     float64(i * 10),
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
			Items: []entity.OrderItem{
				{ProductID: uint64(i), ProductNameSnapshot: "Item", Quantity: 1, UnitPriceSnapshot: 10, LineTotal: 10},
			},
		}
		require.NoError(t, repo.CreateOrderWithItems(context.Background(), o))
	}

	for i := 1; i <= 2; i++ {
		o := &entity.Order{
			UserID:    2,
			Status:    entity.OrderStatusPending,
			Total:     float64(i * 10),
			CreatedAt: now.Add(time.Duration(i) * time.Minute),
			Items: []entity.OrderItem{
				{ProductID: uint64(i), ProductNameSnapshot: "Item", Quantity: 1, UnitPriceSnapshot: 10, LineTotal: 10},
			},
		}
		require.NoError(t, repo.CreateOrderWithItems(context.Background(), o))
	}

	// Fetch page 1 (limit 2) for user 1
	ordersPage1, total, err := repo.GetOrdersByUserID(context.Background(), 1, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, ordersPage1, 2)
	// Newest first -> order with i=5 should be first
	assert.Equal(t, 50.0, ordersPage1[0].Total)
	assert.Equal(t, 40.0, ordersPage1[1].Total)

	// Fetch page 2 (limit 2) for user 1
	ordersPage2, _, err := repo.GetOrdersByUserID(context.Background(), 1, 2, 2)
	require.NoError(t, err)
	assert.Len(t, ordersPage2, 2)
	assert.Equal(t, 30.0, ordersPage2[0].Total)
	assert.Equal(t, 20.0, ordersPage2[1].Total)

	// Verify user 2 gets only their 2 orders
	ordersUser2, totalUser2, err := repo.GetOrdersByUserID(context.Background(), 2, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), totalUser2)
	assert.Len(t, ordersUser2, 2)
}

func TestOrderRepository_GetOrderByID_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewOrderRepository(db)

	_, err := repo.GetOrderByID(context.Background(), 9999)
	assert.ErrorIs(t, err, repository.ErrOrderNotFound)
}

func TestOrderRepository_UpdateOrderStatus_StateTransition(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewOrderRepository(db)

	order := &entity.Order{
		UserID: 1,
		Status: entity.OrderStatusPending,
		Total:  20.0,
		Items: []entity.OrderItem{
			{ProductID: 1, ProductNameSnapshot: "Item", Quantity: 1, UnitPriceSnapshot: 20, LineTotal: 20},
		},
	}
	require.NoError(t, repo.CreateOrderWithItems(context.Background(), order))

	// Transition PENDING -> CANCELLED: should succeed
	err := repo.UpdateOrderStatus(context.Background(), order.ID, entity.OrderStatusPending, entity.OrderStatusCancelled)
	require.NoError(t, err)

	// Verify updated status in DB
	updated, err := repo.GetOrderByID(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.OrderStatusCancelled, updated.Status)

	// Attempt repeat transition PENDING -> CANCELLED: should return ErrOrderStateMismatch
	errRepeat := repo.UpdateOrderStatus(context.Background(), order.ID, entity.OrderStatusPending, entity.OrderStatusCancelled)
	assert.ErrorIs(t, errRepeat, repository.ErrOrderStateMismatch)
}

func TestOrderRepository_CreateOrderWithOutbox_Success(t *testing.T) {
	db := setupTestDB(t)
	orderRepo := repository.NewOrderRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)

	order := &entity.Order{
		UserID: 42,
		Status: entity.OrderStatusPending,
		Total:  99.99,
		Items: []entity.OrderItem{
			{ProductID: 1, ProductNameSnapshot: "Keyboard", Quantity: 1, UnitPriceSnapshot: 99.99, LineTotal: 99.99},
		},
	}

	var capturedOrderID uint64
	err := orderRepo.CreateOrderWithOutbox(context.Background(), order, func(orderID uint64) (*entity.OutboxEvent, error) {
		capturedOrderID = orderID
		return &entity.OutboxEvent{
			EventID:    "evt-order-created-1",
			EventType:  "order.created",
			Exchange:   "order.events",
			RoutingKey: "order.created",
			Payload:    []byte(`{"order_id":1001}`),
			Status:     entity.OutboxStatusPending,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}, nil
	})
	require.NoError(t, err)

	assert.NotZero(t, order.ID)
	assert.Equal(t, order.ID, capturedOrderID)

	// Verify order exists
	savedOrder, err := orderRepo.GetOrderByID(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, uint(42), savedOrder.UserID)

	// Verify outbox record exists and was persisted in same tx
	savedOutbox, err := outboxRepo.GetEventByEventID(context.Background(), "evt-order-created-1")
	require.NoError(t, err)
	assert.Equal(t, entity.OutboxStatusPending, savedOutbox.Status)
	assert.Equal(t, "order.created", savedOutbox.EventType)
}

func TestOrderRepository_CreateOrderWithOutbox_AtomicRollback(t *testing.T) {
	db := setupTestDB(t)
	orderRepo := repository.NewOrderRepository(db)

	order := &entity.Order{
		UserID: 42,
		Status: entity.OrderStatusPending,
		Total:  99.99,
		Items: []entity.OrderItem{
			{ProductID: 1, ProductNameSnapshot: "Keyboard", Quantity: 1, UnitPriceSnapshot: 99.99, LineTotal: 99.99},
		},
	}

	simulatedErr := assert.AnError
	err := orderRepo.CreateOrderWithOutbox(context.Background(), order, func(orderID uint64) (*entity.OutboxEvent, error) {
		return nil, simulatedErr
	})
	assert.ErrorIs(t, err, simulatedErr)

	// Verify that the order was rolled back and DOES NOT exist in the database
	var orderCount int64
	db.Model(&entity.Order{}).Count(&orderCount)
	assert.Equal(t, int64(0), orderCount)

	var itemCount int64
	db.Model(&entity.OrderItem{}).Count(&itemCount)
	assert.Equal(t, int64(0), itemCount)

	var outboxCount int64
	db.Model(&entity.OutboxEvent{}).Count(&outboxCount)
	assert.Equal(t, int64(0), outboxCount)
}

func TestOrderRepository_UpdateOrderStatusWithOutbox_Success(t *testing.T) {
	db := setupTestDB(t)
	orderRepo := repository.NewOrderRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)

	order := &entity.Order{
		UserID: 1,
		Status: entity.OrderStatusPending,
		Total:  20.0,
		Items: []entity.OrderItem{
			{ProductID: 1, ProductNameSnapshot: "Item", Quantity: 1, UnitPriceSnapshot: 20, LineTotal: 20},
		},
	}
	require.NoError(t, orderRepo.CreateOrderWithItems(context.Background(), order))

	err := orderRepo.UpdateOrderStatusWithOutbox(context.Background(), order.ID, entity.OrderStatusPending, entity.OrderStatusCancelled, func() (*entity.OutboxEvent, error) {
		return &entity.OutboxEvent{
			EventID:    "evt-cancel-1",
			EventType:  "order.cancelled",
			Exchange:   "order.events",
			RoutingKey: "order.cancelled",
			Payload:    []byte(`{"order_id":` + string(rune('0'+order.ID)) + `}`),
			Status:     entity.OutboxStatusPending,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}, nil
	})
	require.NoError(t, err)

	// Verify order status updated
	updated, err := orderRepo.GetOrderByID(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.OrderStatusCancelled, updated.Status)

	// Verify outbox event persisted
	ev, err := outboxRepo.GetEventByEventID(context.Background(), "evt-cancel-1")
	require.NoError(t, err)
	assert.Equal(t, "order.cancelled", ev.EventType)
	assert.Equal(t, entity.OutboxStatusPending, ev.Status)
}

func TestOrderRepository_UpdateOrderStatusWithOutbox_Rollback(t *testing.T) {
	db := setupTestDB(t)
	orderRepo := repository.NewOrderRepository(db)
	outboxRepo := repository.NewOutboxRepository(db)

	order := &entity.Order{
		UserID: 1,
		Status: entity.OrderStatusPending,
		Total:  20.0,
		Items: []entity.OrderItem{
			{ProductID: 1, ProductNameSnapshot: "Item", Quantity: 1, UnitPriceSnapshot: 20, LineTotal: 20},
		},
	}
	require.NoError(t, orderRepo.CreateOrderWithItems(context.Background(), order))

	simErr := assert.AnError
	err := orderRepo.UpdateOrderStatusWithOutbox(context.Background(), order.ID, entity.OrderStatusPending, entity.OrderStatusCancelled, func() (*entity.OutboxEvent, error) {
		return nil, simErr
	})
	assert.ErrorIs(t, err, simErr)

	// Verify status remained PENDING (not cancelled!)
	fetched, err := orderRepo.GetOrderByID(context.Background(), order.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.OrderStatusPending, fetched.Status)

	// Verify no outbox event was saved
	_, err = outboxRepo.GetEventByEventID(context.Background(), "evt-cancel-1")
	assert.ErrorIs(t, err, repository.ErrOutboxEventNotFound)
}
