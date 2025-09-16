package orders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/dzuura/satu-lemari/domain/common"
	"github.com/dzuura/satu-lemari/domain/config"
	"github.com/dzuura/satu-lemari/domain/database"
	appError "github.com/dzuura/satu-lemari/domain/error"
	"github.com/dzuura/satu-lemari/domain/models"
)

type OrdersService struct {
	config *config.Config
	db     *database.Database
}

func NewOrdersService(cfg *config.Config, db *database.Database) *OrdersService {
	return &OrdersService{config: cfg, db: db}
}

// getUserCoords fetches latitude/longitude for a given user from Supabase users table
func (s *OrdersService) getUserCoords(ctx context.Context, userID string) (float64, float64, error) {
	url := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=latitude,longitude", s.config.SupabaseURL, userID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("failed to fetch user coords, status %d", resp.StatusCode)
	}
	var arr []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil || len(arr) == 0 {
		return 0, 0, fmt.Errorf("invalid coords response")
	}
	lat, _ := arr[0]["latitude"].(float64)
	lng, _ := arr[0]["longitude"].(float64)
	return lat, lng, nil
}

// haversineKm computes distance in KM between two coordinates
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0 // km
	toRad := func(d float64) float64 { return d * (math.Pi / 180.0) }
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

// CreateOrder handles POST /orders for thrifting purchases
func (s *OrdersService) CreateOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	var req models.CreateOrderRequest
	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Invalid request body"), common.GenerateTraceID())
		return
	}

	if req.ItemID == uuid.Nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrMissingField, "item_id is required"), common.GenerateTraceID())
		return
	}
	if !common.Contains([]string{models.ShippingMethodDirectCOD, models.ShippingMethodAppAgent, models.ShippingMethodPickupWarehouse}, req.ShippingMethod) {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Invalid shipping_method"), common.GenerateTraceID())
		return
	}

	// Validate seller delivery choice for pickup_warehouse method
	if req.ShippingMethod == models.ShippingMethodPickupWarehouse {
		if req.SellerDeliveryChoice == nil {
			appError.WriteErrorResponse(w, appError.New(appError.ErrMissingField, "seller_delivery_choice is required for pickup_warehouse method"), common.GenerateTraceID())
			return
		}
		if !common.Contains([]string{models.SellerDeliverySelfDeliver, models.SellerDeliveryAgentPickup}, *req.SellerDeliveryChoice) {
			appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "seller_delivery_choice must be 'self_deliver' or 'agent_pickup'"), common.GenerateTraceID())
			return
		}
	}

	// Fetch item minimal fields from Supabase
	itemURL := fmt.Sprintf("%s/rest/v1/items?id=eq.%s&select=id,partner_id,type,price,available_quantity,status", s.config.SupabaseURL, req.ItemID.String())
	itemInfo, appErr := s.getSingle(itemURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Validate item availability and type
	itemType, _ := itemInfo["type"].(string)
	partnerID, _ := itemInfo["partner_id"].(string)
	status, _ := itemInfo["status"].(string)
	priceVal := 0.0
	if v, ok := itemInfo["price"].(float64); ok {
		priceVal = v
	}
	available := 0.0
	if v, ok := itemInfo["available_quantity"].(float64); ok {
		available = v
	}

	if status != "active" || int(available) < 1 {
		appError.WriteErrorResponse(w, appError.New(appError.ErrItemNotAvailable, "Item is not available"), common.GenerateTraceID())
		return
	}
	// Validate item type and shipping method compatibility
	if itemType == "rental" && req.ShippingMethod != models.ShippingMethodDirectCOD {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Rental items can only use direct_cod shipping method"), common.GenerateTraceID())
		return
	}
	if !common.Contains([]string{"donation", "rental", "thrifting"}, itemType) {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Invalid item type for ordering"), common.GenerateTraceID())
		return
	}

	// Calculate distance automatically using user and warehouse coordinates
	var totalDistanceForBuyer float64
	if req.ShippingMethod != models.ShippingMethodDirectCOD {
		// Read single warehouse location from config
		warehouseLat := s.config.WarehouseLat
		warehouseLng := s.config.WarehouseLng

		buyerLat, buyerLng, berr := s.getUserCoords(r.Context(), userID)
		if berr != nil {
			appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Buyer location not available"), common.GenerateTraceID())
			return
		}
		sellerLat, sellerLng, serr := s.getUserCoords(r.Context(), partnerID)
		if serr != nil {
			appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Seller location not available"), common.GenerateTraceID())
			return
		}

		distanceBuyerToWarehouse := haversineKm(buyerLat, buyerLng, warehouseLat, warehouseLng)
		distanceSellerToWarehouse := haversineKm(sellerLat, sellerLng, warehouseLat, warehouseLng)

		switch req.ShippingMethod {
		case models.ShippingMethodAppAgent:
			// App agent: charge buyer for both legs
			totalDistanceForBuyer = distanceSellerToWarehouse + distanceBuyerToWarehouse
		case models.ShippingMethodPickupWarehouse:
			// Buyer picks up: buyer distance is zero
			totalDistanceForBuyer = 0
		}
	}

	// Calculate shipping fee based on item type and seller delivery choice
	shippingFee := s.calculateShippingFee(itemType, req.ShippingMethod, &totalDistanceForBuyer, req.SellerDeliveryChoice)

	// For donation items, price is always 0
	if itemType == "donation" {
		priceVal = 0
	}

	total := priceVal + shippingFee
	expiresAt := time.Now().Add(24 * time.Hour)

	// Build order payload for insert
	orderPayload := map[string]interface{}{
		"item_id":         req.ItemID,
		"buyer_id":        userID,
		"seller_id":       partnerID,
		"type":            itemType,
		"shipping_method": req.ShippingMethod,
		"status":          models.OrderStatusAwaitingPayment,
		"item_price":      priceVal,
		"shipping_fee":    shippingFee,
		"total_amount":    total,
		"weight_kg":       req.WeightKg,
		"distance_km":     totalDistanceForBuyer,
		"notes":           req.Notes,
		"expires_at":      expiresAt,
	}

	rows, err := s.db.Insert(r.Context(), "orders", []map[string]interface{}{orderPayload})
	if err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrDatabase, "Failed to create order"), common.GenerateTraceID())
		return
	}

	var createdOrders []map[string]interface{}
	if err := json.Unmarshal(rows.Data(), &createdOrders); err != nil || len(createdOrders) == 0 {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInternal, "Failed to parse order response"), common.GenerateTraceID())
		return
	}
	orderIDStr, _ := createdOrders[0]["id"].(string)
	orderID, _ := uuid.Parse(orderIDStr)

	// Create payment (pending) with QRIS payload placeholder
	qrisPayload := fmt.Sprintf("qris://pay?order_id=%s", orderID.String())
	paymentPayload := map[string]interface{}{
		"order_id":     orderID,
		"method":       "qris",
		"status":       models.PaymentStatusPending,
		"amount":       total,
		"qris_payload": qrisPayload,
	}
	if _, err := s.db.Insert(r.Context(), "payments", []map[string]interface{}{paymentPayload}); err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrDatabase, "Failed to create payment"), common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"order_id":     orderID.String(),
		"status":       models.OrderStatusAwaitingPayment,
		"item_price":   priceVal,
		"shipping_fee": shippingFee,
		"total_amount": total,
		"qris": map[string]interface{}{
			"method":  "qris",
			"payload": qrisPayload,
		},
		"expires_at": expiresAt,
	}

	common.WriteSuccessResponse(w, response, "Order created successfully")
}

func (s *OrdersService) calculateShippingFee(itemType, method string, distanceKm *float64, _ *string) float64 {
	// direct COD => 0 from app side
	if method == models.ShippingMethodDirectCOD {
		return 0
	}

	// For rental items, only direct COD is allowed (already validated above)
	if itemType == "rental" {
		return 0
	}

	if distanceKm == nil {
		return 0
	}

	d := *distanceKm
	if d <= 2.0 {
		return 0
	}

	extraKm := math.Ceil(d - 2.0)
	shippingFee := extraKm * 3000

	// For donation items, shipping fee is always 0 for buyer (seller pays)
	if itemType == "donation" {
		return 0
	}

	// For pickup_warehouse method, buyer pays 0 (seller handles delivery to warehouse)
	if method == models.ShippingMethodPickupWarehouse {
		return 0
	}

	// For thrifting items with app_agent method, buyer pays shipping fee
	return shippingFee
}

// getSingle queries Supabase REST and returns the first object as map
func (s *OrdersService) getSingle(url string) (map[string]interface{}, *appError.AppError) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to build request")
	}
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Database request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, appError.New(appError.ErrDatabase, "Database query failed")
	}
	var arr []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil || len(arr) == 0 {
		return nil, appError.New(appError.ErrInternal, "Failed to parse database response")
	}
	return arr[0], nil
}

// getMultiple queries Supabase REST and returns array of objects
func (s *OrdersService) getMultiple(url string) ([]map[string]interface{}, *appError.AppError) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to build request")
	}
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, appError.New(appError.ErrDatabase, "Database request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, appError.New(appError.ErrDatabase, "Database query failed")
	}
	var arr []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse database response")
	}
	return arr, nil
}

// ExpireOrders handles POST /orders/expire (admin only) - auto-expire unpaid orders after 24 hours
func (s *OrdersService) ExpireOrders(w http.ResponseWriter, r *http.Request) {
	// Find orders that are awaiting payment and expired
	expiredOrdersURL := fmt.Sprintf("%s/rest/v1/orders?status=eq.%s&expires_at=lt.%s&select=id",
		s.config.SupabaseURL,
		models.OrderStatusAwaitingPayment,
		time.Now().Format(time.RFC3339))

	expiredOrders, appErr := s.getMultiple(expiredOrdersURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	if len(expiredOrders) == 0 {
		common.WriteSuccessResponse(w, map[string]interface{}{
			"expired_count": 0,
			"message":       "No expired orders found",
		}, "No orders to expire")
		return
	}

	// Update orders status to expired
	expiredCount := 0
	for _, order := range expiredOrders {
		id, _ := order["id"].(string)
		if id == "" {
			continue
		}
		orderURL := fmt.Sprintf("%s/rest/v1/orders?id=eq.%s", s.config.SupabaseURL, id)
		if err := s.updateRecord(orderURL, map[string]interface{}{"status": models.OrderStatusExpired}); err == nil {
			expiredCount++
		}
	}

	common.WriteSuccessResponse(w, map[string]interface{}{
		"expired_count": expiredCount,
	}, "Orders expiration completed")
}

// VerifyPayment handles POST /orders/{order_id}/verify-payment (admin only)
func (s *OrdersService) VerifyPayment(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	// Get order ID from URL
	vars := mux.Vars(r)
	orderIDStr := vars["order_id"]
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Invalid order ID"), common.GenerateTraceID())
		return
	}

	var req models.VerifyPaymentRequest
	if err := common.ParseJSONBody(r, &req); err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Invalid request body"), common.GenerateTraceID())
		return
	}

	if !common.Contains([]string{models.PaymentStatusPaid, models.PaymentStatusRejected}, req.Status) {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Status must be 'paid' or 'rejected'"), common.GenerateTraceID())
		return
	}

	// Check if order exists and get current status + item
	orderURL := fmt.Sprintf("%s/rest/v1/orders?id=eq.%s&select=id,status,item_id,type", s.config.SupabaseURL, orderID.String())
	orderInfo, appErr := s.getSingle(orderURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrNotFound, "Order not found"), common.GenerateTraceID())
		return
	}

	currentStatus, _ := orderInfo["status"].(string)
	orderType, _ := orderInfo["type"].(string)
	itemIDStr, _ := orderInfo["item_id"].(string)

	if currentStatus != models.OrderStatusAwaitingPayment {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Order is not awaiting payment"), common.GenerateTraceID())
		return
	}

	// Update payment status
	paymentUpdate := map[string]interface{}{
		"status":      req.Status,
		"verified_by": userID,
		"verified_at": time.Now(),
	}
	if req.Status == models.PaymentStatusPaid {
		paymentUpdate["paid_at"] = time.Now()
	}

	paymentURL := fmt.Sprintf("%s/rest/v1/payments?order_id=eq.%s", s.config.SupabaseURL, orderID.String())
	if err := s.updateRecord(paymentURL, paymentUpdate); err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrDatabase, "Failed to update payment"), common.GenerateTraceID())
		return
	}

	// Update order status based on payment result
	var newOrderStatus string
	if req.Status == models.PaymentStatusPaid {
		newOrderStatus = models.OrderStatusPaid
	} else {
		newOrderStatus = models.OrderStatusCancelled
	}

	orderUpdate := map[string]interface{}{
		"status": newOrderStatus,
	}
	if err := s.updateRecord(orderURL, orderUpdate); err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrDatabase, "Failed to update order"), common.GenerateTraceID())
		return
	}

	// If thrifting and paid: decrease item stock by 1
	if newOrderStatus == models.OrderStatusPaid && orderType == "thrifting" && itemIDStr != "" {
		_ = s.decrementItemStock(context.Background(), itemIDStr, 1)
	}

	response := map[string]interface{}{
		"order_id": orderID.String(),
		"status":   newOrderStatus,
		"payment": map[string]interface{}{
			"status":      req.Status,
			"verified_by": userID,
			"verified_at": time.Now(),
		},
	}

	common.WriteSuccessResponse(w, response, "Payment verification completed")
}

func (s *OrdersService) decrementItemStock(_ context.Context, itemID string, qty int) error {
	// Get current available_quantity
	getURL := fmt.Sprintf("%s/rest/v1/items?id=eq.%s&select=available_quantity", s.config.SupabaseURL, itemID)
	info, appErr := s.getSingle(getURL)
	if appErr != nil {
		return fmt.Errorf("failed to fetch item: %v", appErr)
	}
	availFloat, _ := info["available_quantity"].(float64)
	newAvail := int(availFloat) - qty
	if newAvail < 0 {
		newAvail = 0
	}
	upd := map[string]interface{}{
		"available_quantity": newAvail,
	}
	patchURL := fmt.Sprintf("%s/rest/v1/items?id=eq.%s", s.config.SupabaseURL, itemID)
	return s.updateRecord(patchURL, upd)
}

// updateRecord updates a single record in Supabase
func (s *OrdersService) updateRecord(url string, data map[string]interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("update failed with status: %d", resp.StatusCode)
	}

	return nil
}
