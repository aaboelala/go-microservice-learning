-- Migration: Drop orders and order_items tables
-- Version: 000001_down

DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;

