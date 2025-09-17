package orders

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
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

	// For donation items, check if there's an approved request instead of available quantity
	var approvedRequestID *string
	if itemType == "donation" {
		if status != "active" {
			appError.WriteErrorResponse(w, appError.New(appError.ErrItemNotAvailable, "Item is not available"), common.GenerateTraceID())
			return
		}
		// Check if there's an approved request for this item and user
		requestURL := fmt.Sprintf("%s/rest/v1/requests?item_id=eq.%s&user_id=eq.%s&status=eq.approved&deleted_by_user=eq.false&deleted_by_partner=eq.false", s.config.SupabaseURL, req.ItemID.String(), userID)
		requests, appErr := s.getMultiple(requestURL)
		if appErr != nil || len(requests) == 0 {
			appError.WriteErrorResponse(w, appError.New(appError.ErrItemNotAvailable, "No approved donation request found for this item"), common.GenerateTraceID())
			return
		}
		// Get the request ID for linking
		if requestID, ok := requests[0]["id"].(string); ok {
			approvedRequestID = &requestID
		}
	} else {
		// For rental and thrifting items, check available quantity
		if status != "active" || int(available) < 1 {
			appError.WriteErrorResponse(w, appError.New(appError.ErrItemNotAvailable, "Item is not available"), common.GenerateTraceID())
			return
		}
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

	// Calculate shipping fee for buyer
	buyerShippingFee := s.calculateShippingFee(itemType, req.ShippingMethod, &totalDistanceForBuyer, req.WeightKg)

	// Calculate shipping fee for seller (from seller to warehouse)
	var sellerShippingFee float64
	if req.ShippingMethod == models.ShippingMethodAppAgent ||
		(req.ShippingMethod == models.ShippingMethodPickupWarehouse &&
			req.SellerDeliveryChoice != nil && *req.SellerDeliveryChoice == models.SellerDeliveryAgentPickup) {
		// Calculate distance from seller to warehouse
		warehouseLat := s.config.WarehouseLat
		warehouseLng := s.config.WarehouseLng
		sellerLat, sellerLng, serr := s.getUserCoords(r.Context(), partnerID)
		if serr == nil {
			distanceSellerToWarehouse := haversineKm(sellerLat, sellerLng, warehouseLat, warehouseLng)
			sellerShippingFee = s.calculateSellerShippingFee(&distanceSellerToWarehouse, req.WeightKg)
		}
	}

	// For donation items, price is always 0
	if itemType == "donation" {
		priceVal = 0
	}

	total := priceVal + buyerShippingFee
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
		"shipping_fee":    buyerShippingFee,
		"total_amount":    total,
		"weight_kg":       req.WeightKg,
		"distance_km":     totalDistanceForBuyer,
		"notes":           req.Notes,
		"expires_at":      expiresAt,
	}

	// Add request_id for donation items
	if approvedRequestID != nil {
		orderPayload["request_id"] = *approvedRequestID
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
		"shipping_fee": buyerShippingFee,
		"total_amount": total,
		"shipping_details": map[string]interface{}{
			"buyer_fee":  buyerShippingFee,
			"seller_fee": sellerShippingFee,
			"method":     req.ShippingMethod,
		},
		"qris": map[string]interface{}{
			"method":  "qris",
			"payload": qrisPayload,
		},
		"expires_at": expiresAt,
	}

	common.WriteSuccessResponse(w, response, "Order created successfully")
}

// calculateShippingFee calculates shipping fee based on new zone-based system
func (s *OrdersService) calculateShippingFee(itemType, method string, distanceKm *float64, weightKg *float64) float64 {
	// Direct COD is always free
	if method == models.ShippingMethodDirectCOD {
		return 0
	}

	// For rental items, only direct COD is allowed (already validated above)
	if itemType == "rental" {
		return 0
	}

	// For pickup_warehouse method, buyer pays 0 (seller handles delivery to warehouse)
	if method == models.ShippingMethodPickupWarehouse {
		return 0
	}

	// For donation items, shipping fee is always 0 for buyer (seller pays)
	if itemType == "donation" {
		return 0
	}

	// Only app_agent method charges buyer
	if method != models.ShippingMethodAppAgent {
		return 0
	}

	if distanceKm == nil {
		return 0
	}

	// Default weight 1kg if not specified
	weight := 1.0
	if weightKg != nil && *weightKg > 0 {
		weight = *weightKg
	}

	distance := *distanceKm
	if distance <= 0 {
		return 0
	}

	// Calculate base rate based on zone
	baseRate := s.getBaseRate(distance)

	// Calculate weight multiplier
	weightMultiplier := s.getWeightMultiplier(weight)

	// Calculate shipping fee
	shippingFee := distance * baseRate * weightMultiplier

	// Round to nearest Rp 500
	roundedFee := s.roundToNearest500(shippingFee)

	return roundedFee
}

// getBaseRate returns base rate per km based on distance zone
func (s *OrdersService) getBaseRate(distanceKm float64) float64 {
	switch {
	case distanceKm <= 10:
		return 1000 // Zone 1: 0-10km
	case distanceKm <= 25:
		return 2000 // Zone 2: 10-25km
	case distanceKm <= 50:
		return 3000 // Zone 3: 25-50km
	default:
		return 4000 // Zone 4: >50km
	}
}

// getWeightMultiplier returns weight multiplier based on weight
func (s *OrdersService) getWeightMultiplier(weightKg float64) float64 {
	switch {
	case weightKg <= 1:
		return 1.0 // 0-1kg: 1x
	case weightKg <= 3:
		return 1.5 // 1-3kg: 1.5x
	case weightKg <= 5:
		return 2.0 // 3-5kg: 2x
	case weightKg <= 10:
		return 3.0 // 5-10kg: 3x
	default:
		return 4.0 // >10kg: 4x
	}
}

// roundToNearest500 rounds amount to nearest Rp 500
func (s *OrdersService) roundToNearest500(amount float64) float64 {
	return math.Ceil(amount/500) * 500
}

// calculateSellerShippingFee calculates shipping fee for seller (from seller to warehouse)
func (s *OrdersService) calculateSellerShippingFee(distanceKm *float64, weightKg *float64) float64 {
	if distanceKm == nil || *distanceKm <= 0 {
		return 0
	}

	// Default weight 1kg if not specified
	weight := 1.0
	if weightKg != nil && *weightKg > 0 {
		weight = *weightKg
	}

	distance := *distanceKm

	// Calculate base rate based on zone
	baseRate := s.getBaseRate(distance)

	// Calculate weight multiplier
	weightMultiplier := s.getWeightMultiplier(weight)

	// Calculate shipping fee
	shippingFee := distance * baseRate * weightMultiplier

	// Round to nearest Rp 500
	roundedFee := s.roundToNearest500(shippingFee)

	return roundedFee
}

// ListOrders handles GET /orders (protected) - get all orders with filters
func (s *OrdersService) ListOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	// Build filters
	q := url.Values{}
	q.Set("select", "*")

	status := r.URL.Query().Get("status")
	orderType := r.URL.Query().Get("type")
	buyerID := r.URL.Query().Get("buyer_id")
	sellerID := r.URL.Query().Get("seller_id")
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	// For regular users, only show their own orders
	// For admin users, show all orders
	// Get user role from database
	userRole := "user" // default
	userURL := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=role", s.config.SupabaseURL, userID)
	userInfo, appErr := s.getSingle(userURL)
	if appErr == nil {
		if role, ok := userInfo["role"].(string); ok {
			userRole = role
		}
	}

	if userRole != "admin" {
		// Regular user: only show orders where they are buyer or seller
		q.Set("or", fmt.Sprintf("(buyer_id.eq.%s,seller_id.eq.%s)", userID, userID))
	}

	if status != "" {
		q.Set("status", "eq."+status)
	}
	if orderType != "" {
		q.Set("type", "eq."+orderType)
	}
	if buyerID != "" && userRole == "admin" {
		q.Set("buyer_id", "eq."+buyerID)
	}
	if sellerID != "" && userRole == "admin" {
		q.Set("seller_id", "eq."+sellerID)
	}

	limit := 20
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	page := 1
	if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
		page = v
	}
	offset := (page - 1) * limit
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("order", "created_at.desc")

	listURL := fmt.Sprintf("%s/rest/v1/orders?%s", s.config.SupabaseURL, q.Encode())
	orders, appErr := s.getMultiple(listURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Total count
	countURL := fmt.Sprintf("%s/rest/v1/orders?select=count", s.config.SupabaseURL)
	if userRole != "admin" {
		countURL = fmt.Sprintf("%s/rest/v1/orders?select=count&or=(buyer_id.eq.%s,seller_id.eq.%s)", s.config.SupabaseURL, userID, userID)
	}
	if status != "" || orderType != "" || (buyerID != "" && userRole == "admin") || (sellerID != "" && userRole == "admin") {
		// reapply filters for count
		cq := url.Values{}
		cq.Set("select", "count")
		if userRole != "admin" {
			cq.Set("or", fmt.Sprintf("(buyer_id.eq.%s,seller_id.eq.%s)", userID, userID))
		}
		if status != "" {
			cq.Set("status", "eq."+status)
		}
		if orderType != "" {
			cq.Set("type", "eq."+orderType)
		}
		if buyerID != "" && userRole == "admin" {
			cq.Set("buyer_id", "eq."+buyerID)
		}
		if sellerID != "" && userRole == "admin" {
			cq.Set("seller_id", "eq."+sellerID)
		}
		countURL = fmt.Sprintf("%s/rest/v1/orders?%s", s.config.SupabaseURL, cq.Encode())
	}
	req, err := http.NewRequest(http.MethodGet, countURL, nil)
	if err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInternal, "Failed to build count request"), common.GenerateTraceID())
		return
	}
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrDatabase, "Count request failed"), common.GenerateTraceID())
		return
	}
	defer resp.Body.Close()
	var countArr []map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&countArr)
	total := len(orders)
	if len(countArr) > 0 {
		if c, ok := countArr[0]["count"].(float64); ok {
			total = int(c)
		}
	}

	meta := common.CalculateMeta(page, limit, total)
	common.WriteSuccessResponseWithMeta(w, orders, meta, "Orders retrieved successfully")
}

// GetOrder handles GET /orders/{order_id} (protected) - get specific order details
func (s *OrdersService) GetOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	vars := mux.Vars(r)
	orderID := vars["order_id"]
	if orderID == "" {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Invalid order ID"), common.GenerateTraceID())
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(orderID); err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Invalid order ID format"), common.GenerateTraceID())
		return
	}

	orderURL := fmt.Sprintf("%s/rest/v1/orders?id=eq.%s&select=*", s.config.SupabaseURL, orderID)
	ord, appErr := s.getSingle(orderURL)
	if appErr != nil {
		// Check if it's a "not found" error specifically
		if appErr.Type == appError.ErrNotFound {
			appError.WriteErrorResponse(w, appError.New(appError.ErrNotFound, "Order not found"), common.GenerateTraceID())
			return
		}
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Check if user is buyer or seller
	buyerID, _ := ord["buyer_id"].(string)
	sellerID, _ := ord["seller_id"].(string)
	if userID != buyerID && userID != sellerID {
		appError.WriteErrorResponse(w, appError.New(appError.ErrForbidden, "You can only view your own orders"), common.GenerateTraceID())
		return
	}

	paymentURL := fmt.Sprintf("%s/rest/v1/payments?order_id=eq.%s&select=*&order=created_at.desc", s.config.SupabaseURL, orderID)
	pmts, appErr := s.getMultiple(paymentURL)
	if appErr != nil {
		// still return order without payment
		common.WriteSuccessResponse(w, map[string]interface{}{"order": ord}, "Order retrieved successfully")
		return
	}
	resp := map[string]interface{}{
		"order":   ord,
		"payment": nil,
	}
	if len(pmts) > 0 {
		resp["payment"] = pmts[0]
	}
	common.WriteSuccessResponse(w, resp, "Order retrieved successfully")
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
	if err := json.NewDecoder(resp.Body).Decode(&arr); err != nil {
		return nil, appError.New(appError.ErrInternal, "Failed to parse database response")
	}
	if len(arr) == 0 {
		return nil, appError.New(appError.ErrNotFound, "Record not found")
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

// ExpireOrders handles POST /orders/expire (admin only) - auto-expire unpaid orders after 24 hours
func (s *OrdersService) ExpireOrders(w http.ResponseWriter, r *http.Request) {
	// Encode timestamp to avoid PostgREST filter issues
	now := time.Now().Format(time.RFC3339)
	nowEsc := url.QueryEscape(now)

	expiredURL := fmt.Sprintf("%s/rest/v1/orders?status=eq.%s&expires_at=lt.%s&select=id", s.config.SupabaseURL, models.OrderStatusAwaitingPayment, nowEsc)
	rows, appErr := s.getMultiple(expiredURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	if len(rows) == 0 {
		common.WriteSuccessResponse(w, map[string]interface{}{
			"expired_count": 0,
			"message":       "No expired orders found",
		}, "No orders to expire")
		return
	}

	expiredCount := 0
	for _, row := range rows {
		id, _ := row["id"].(string)
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

// GetMyOrders handles GET /orders/my (protected) - get current user's orders (USER ROLE ONLY)
func (s *OrdersService) GetMyOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	// Check if user has 'user' role
	userURL := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=role", s.config.SupabaseURL, userID)
	userInfo, appErr := s.getSingle(userURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrNotFound, "User not found"), common.GenerateTraceID())
		return
	}

	userRole, _ := userInfo["role"].(string)
	if userRole != "user" {
		appError.WriteErrorResponse(w, appError.New(appError.ErrForbidden, "This endpoint is only accessible by users"), common.GenerateTraceID())
		return
	}

	// Build filters - show orders where this user is the buyer
	q := url.Values{}
	q.Set("select", "*")
	q.Set("buyer_id", "eq."+userID)

	status := r.URL.Query().Get("status")
	orderType := r.URL.Query().Get("type")
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	if status != "" {
		q.Set("status", "eq."+status)
	}
	if orderType != "" {
		q.Set("type", "eq."+orderType)
	}

	limit := 20
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	page := 1
	if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
		page = v
	}
	offset := (page - 1) * limit
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("order", "created_at.desc")

	listURL := fmt.Sprintf("%s/rest/v1/orders?%s", s.config.SupabaseURL, q.Encode())
	orders, appErr := s.getMultiple(listURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Total count
	countURL := fmt.Sprintf("%s/rest/v1/orders?select=count&buyer_id=eq.%s", s.config.SupabaseURL, userID)
	if status != "" || orderType != "" {
		// reapply filters for count
		cq := url.Values{}
		cq.Set("select", "count")
		cq.Set("buyer_id", "eq."+userID)
		if status != "" {
			cq.Set("status", "eq."+status)
		}
		if orderType != "" {
			cq.Set("type", "eq."+orderType)
		}
		countURL = fmt.Sprintf("%s/rest/v1/orders?%s", s.config.SupabaseURL, cq.Encode())
	}
	req, err := http.NewRequest(http.MethodGet, countURL, nil)
	if err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInternal, "Failed to build count request"), common.GenerateTraceID())
		return
	}
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrDatabase, "Count request failed"), common.GenerateTraceID())
		return
	}
	defer resp.Body.Close()
	var countArr []map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&countArr)
	total := len(orders)
	if len(countArr) > 0 {
		if c, ok := countArr[0]["count"].(float64); ok {
			total = int(c)
		}
	}

	meta := common.CalculateMeta(page, limit, total)
	common.WriteSuccessResponseWithMeta(w, orders, meta, "My orders retrieved successfully")
}

// GetPartnerOrders handles GET /orders/partner (protected) - get orders for partner's items (PARTNER ROLE ONLY)
func (s *OrdersService) GetPartnerOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	// Check if user has 'partner' role
	userURL := fmt.Sprintf("%s/rest/v1/users?id=eq.%s&select=role", s.config.SupabaseURL, userID)
	userInfo, appErr := s.getSingle(userURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrNotFound, "User not found"), common.GenerateTraceID())
		return
	}

	userRole, _ := userInfo["role"].(string)
	if userRole != "partner" {
		appError.WriteErrorResponse(w, appError.New(appError.ErrForbidden, "This endpoint is only accessible by partners"), common.GenerateTraceID())
		return
	}

	// Build filters - show orders where this partner is the seller (orders for their items)
	q := url.Values{}
	q.Set("select", "*")
	q.Set("seller_id", "eq."+userID)

	status := r.URL.Query().Get("status")
	orderType := r.URL.Query().Get("type")
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	if status != "" {
		q.Set("status", "eq."+status)
	}
	if orderType != "" {
		q.Set("type", "eq."+orderType)
	}

	limit := 20
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	page := 1
	if v, err := strconv.Atoi(pageStr); err == nil && v > 0 {
		page = v
	}
	offset := (page - 1) * limit
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("order", "created_at.desc")

	listURL := fmt.Sprintf("%s/rest/v1/orders?%s", s.config.SupabaseURL, q.Encode())
	orders, appErr := s.getMultiple(listURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appErr, common.GenerateTraceID())
		return
	}

	// Total count
	countURL := fmt.Sprintf("%s/rest/v1/orders?select=count&seller_id=eq.%s", s.config.SupabaseURL, userID)
	if status != "" || orderType != "" {
		// reapply filters for count
		cq := url.Values{}
		cq.Set("select", "count")
		cq.Set("seller_id", "eq."+userID)
		if status != "" {
			cq.Set("status", "eq."+status)
		}
		if orderType != "" {
			cq.Set("type", "eq."+orderType)
		}
		countURL = fmt.Sprintf("%s/rest/v1/orders?%s", s.config.SupabaseURL, cq.Encode())
	}
	req, err := http.NewRequest(http.MethodGet, countURL, nil)
	if err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInternal, "Failed to build count request"), common.GenerateTraceID())
		return
	}
	req.Header.Set("apikey", s.config.SupabaseServiceRoleKey)
	req.Header.Set("Authorization", "Bearer "+s.config.SupabaseServiceRoleKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrDatabase, "Count request failed"), common.GenerateTraceID())
		return
	}
	defer resp.Body.Close()
	var countArr []map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&countArr)
	total := len(orders)
	if len(countArr) > 0 {
		if c, ok := countArr[0]["count"].(float64); ok {
			total = int(c)
		}
	}

	meta := common.CalculateMeta(page, limit, total)
	common.WriteSuccessResponseWithMeta(w, orders, meta, "Partner orders retrieved successfully")
}

// DeleteOrder handles DELETE /orders/{order_id} (protected) - cancel order
func (s *OrdersService) DeleteOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := common.GetUserIDFromContext(r)
	if !ok {
		appError.WriteErrorResponse(w, appError.New(appError.ErrUnauthorized, "Unauthorized"), common.GenerateTraceID())
		return
	}

	vars := mux.Vars(r)
	orderID := vars["order_id"]
	if orderID == "" {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Invalid order ID"), common.GenerateTraceID())
		return
	}

	// Check if order exists and get current status
	orderURL := fmt.Sprintf("%s/rest/v1/orders?id=eq.%s&select=id,status,buyer_id,seller_id", s.config.SupabaseURL, orderID)
	orderInfo, appErr := s.getSingle(orderURL)
	if appErr != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrNotFound, "Order not found"), common.GenerateTraceID())
		return
	}

	currentStatus, _ := orderInfo["status"].(string)
	buyerID, _ := orderInfo["buyer_id"].(string)
	sellerID, _ := orderInfo["seller_id"].(string)

	// Check if user is buyer or seller
	if userID != buyerID && userID != sellerID {
		appError.WriteErrorResponse(w, appError.New(appError.ErrForbidden, "You can only cancel your own orders"), common.GenerateTraceID())
		return
	}

	// Only allow cancellation of pending or awaiting_payment orders
	if currentStatus != models.OrderStatusPending && currentStatus != models.OrderStatusAwaitingPayment {
		appError.WriteErrorResponse(w, appError.New(appError.ErrInvalidInput, "Order cannot be cancelled in current status"), common.GenerateTraceID())
		return
	}

	// Update order status to cancelled
	orderUpdate := map[string]interface{}{
		"status": models.OrderStatusCancelled,
	}
	if err := s.updateRecord(orderURL, orderUpdate); err != nil {
		appError.WriteErrorResponse(w, appError.New(appError.ErrDatabase, "Failed to cancel order"), common.GenerateTraceID())
		return
	}

	response := map[string]interface{}{
		"order_id": orderID,
		"status":   models.OrderStatusCancelled,
		"message":  "Order cancelled successfully",
	}

	common.WriteSuccessResponse(w, response, "Order cancelled successfully")
}
