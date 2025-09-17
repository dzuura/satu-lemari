package models

import (
	"time"

	"github.com/google/uuid"
)

// ShippingMethod enumerations
const (
	ShippingMethodDirectCOD       = "direct_cod"
	ShippingMethodAppAgent        = "app_agent"
	ShippingMethodPickupWarehouse = "pickup_warehouse"
)

// OrderStatus enumerations
const (
	OrderStatusPending         = "pending"
	OrderStatusAwaitingPayment = "awaiting_payment"
	OrderStatusPaid            = "paid"
	OrderStatusProcessing      = "processing"
	OrderStatusShipped         = "shipped"
	OrderStatusDelivered       = "delivered"
	OrderStatusCompleted       = "completed"
	OrderStatusCancelled       = "cancelled"
	OrderStatusExpired         = "expired"
)

// Order represents purchase order (primarily for thrifting)
type Order struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	ItemID         uuid.UUID  `json:"item_id" db:"item_id"`
	BuyerID        string     `json:"buyer_id" db:"buyer_id"`
	SellerID       string     `json:"seller_id" db:"seller_id"`
	RequestID      *uuid.UUID `json:"request_id,omitempty" db:"request_id"`
	Type           string     `json:"type" db:"type"`
	ShippingMethod string     `json:"shipping_method" db:"shipping_method"`
	Status         string     `json:"status" db:"status"`
	ItemPrice      float64    `json:"item_price" db:"item_price"`
	ShippingFee    float64    `json:"shipping_fee" db:"shipping_fee"`
	TotalAmount    float64    `json:"total_amount" db:"total_amount"`
	WeightKg       *float64   `json:"weight_kg,omitempty" db:"weight_kg"`
	DistanceKm     *float64   `json:"distance_km,omitempty" db:"distance_km"`
	Notes          *string    `json:"notes,omitempty" db:"notes"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// CreateOrderRequest payload for creating an order
type CreateOrderRequest struct {
	ItemID         uuid.UUID `json:"item_id" validate:"required"`
	ShippingMethod string    `json:"shipping_method" validate:"required,oneof=direct_cod app_agent pickup_warehouse"`
	WeightKg       *float64  `json:"weight_kg,omitempty"`
	DistanceKm     *float64  `json:"distance_km,omitempty"`
	Notes          *string   `json:"notes,omitempty"`
	// For pickup_warehouse method, seller can choose delivery method
	SellerDeliveryChoice *string `json:"seller_delivery_choice,omitempty"` // "self_deliver" or "agent_pickup"
}

// VerifyPaymentRequest payload for verifying payment
type VerifyPaymentRequest struct {
	Status string `json:"status" validate:"required,oneof=paid rejected"`
}

// Payment represents a payment record for an order
type Payment struct {
	ID           uuid.UUID  `json:"id" db:"id"`
	OrderID      uuid.UUID  `json:"order_id" db:"order_id"`
	Method       string     `json:"method" db:"method"`
	Status       string     `json:"status" db:"status"`
	Amount       float64    `json:"amount" db:"amount"`
	QRISPayload  *string    `json:"qris_payload,omitempty" db:"qris_payload"`
	QRISImageURL *string    `json:"qris_image_url,omitempty" db:"qris_image_url"`
	PaidAt       *time.Time `json:"paid_at,omitempty" db:"paid_at"`
	VerifiedBy   *string    `json:"verified_by,omitempty" db:"verified_by"`
	VerifiedAt   *time.Time `json:"verified_at,omitempty" db:"verified_at"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
}

// PaymentStatus enumerations
const (
	PaymentStatusPending  = "pending"
	PaymentStatusPaid     = "paid"
	PaymentStatusRejected = "rejected"
)

// Shipment represents the shipping record for an order
type Shipment struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	OrderID        uuid.UUID  `json:"order_id" db:"order_id"`
	Method         string     `json:"method" db:"method"`
	Status         string     `json:"status" db:"status"`
	Payer          string     `json:"payer" db:"payer"`
	PickupSlotID   *uuid.UUID `json:"pickup_slot_id,omitempty" db:"pickup_slot_id"`
	DeliverySlotID *uuid.UUID `json:"delivery_slot_id,omitempty" db:"delivery_slot_id"`
	DistanceKm     *float64   `json:"distance_km,omitempty" db:"distance_km"`
	WeightKg       *float64   `json:"weight_kg,omitempty" db:"weight_kg"`
	BaseFee        float64    `json:"base_fee" db:"base_fee"`
	ExtraFee       float64    `json:"extra_fee" db:"extra_fee"`
	TotalFee       float64    `json:"total_fee" db:"total_fee"`
	Notes          *string    `json:"notes,omitempty" db:"notes"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// ShipmentStatus enumerations
const (
	ShipmentStatusPending   = "pending"
	ShipmentStatusScheduled = "scheduled"
	ShipmentStatusPickedUp  = "picked_up"
	ShipmentStatusInTransit = "in_transit"
	ShipmentStatusDelivered = "delivered"
	ShipmentStatusCancelled = "cancelled"
)

// SellerDeliveryChoice enumerations
const (
	SellerDeliverySelfDeliver = "self_deliver" // Penjual antar ke gudang (gratis)
	SellerDeliveryAgentPickup = "agent_pickup" // Agent jemput dari penjual (berbayar)
)
