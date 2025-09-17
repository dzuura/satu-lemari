package database

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/dzuura/satu-lemari/domain/config"
)

// DatabaseInterface defines the interface that repositories expect
type DatabaseInterface interface {
	// Query executes a query that returns rows
	Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)

	// QueryRow executes a query that returns a single row
	QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row

	// Exec executes a query without returning rows
	Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error)

	// BeginTx starts a new transaction
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// Ping checks if the database is accessible
	Ping(ctx context.Context) error

	// Close closes the database connection
	Close() error
}

// Ensure Database implements DatabaseInterface
var _ DatabaseInterface = (*Database)(nil)

// Database represents the Supabase REST API client
type Database struct {
	config     *config.Config
	httpClient *http.Client
}

// NewDatabase creates a new Supabase REST API client
func NewDatabase(cfg *config.Config) (*Database, error) {
	if cfg.SupabaseURL == "" {
		return nil, fmt.Errorf("SUPABASE_URL is required")
	}
	if cfg.SupabaseKey == "" {
		return nil, fmt.Errorf("SUPABASE_KEY is required")
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	db := &Database{
		config:     cfg,
		httpClient: httpClient,
	}

	// Test connection by making a simple query
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to Supabase: %v", err)
	}

	log.Println("Supabase REST API connection established successfully")

	return db, nil
}

// PingContext tests the connection to Supabase
func (d *Database) PingContext(ctx context.Context) error {
	url := fmt.Sprintf("%s/rest/v1/", d.config.SupabaseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", d.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+d.config.SupabaseKey)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to ping Supabase: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Supabase ping failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Exec executes a query without returning rows
func (d *Database) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	// For REST API, we'll use POST to execute queries
	// This is a simplified implementation
	url := fmt.Sprintf("%s/rest/v1/rpc/exec", d.config.SupabaseURL)

	payload := map[string]interface{}{
		"query": query,
		"args":  args,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("apikey", d.config.SupabaseKey)
	req.Header.Set("Authorization", "Bearer "+d.config.SupabaseKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query execution failed with status %d: %s", resp.StatusCode, string(body))
	}

	return &Result{}, nil
}

// Query executes a query that returns rows (compatible with sql.Rows interface)
func (d *Database) Query(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	// For REST API, we need to convert SQL query to REST API call
	// This is a simplified implementation that assumes simple SELECT queries
	// In a real implementation, you would need a SQL parser to convert to REST API

	// For now, return error indicating that direct SQL queries are not supported
	return nil, fmt.Errorf("direct SQL queries not supported in REST API mode. Use table-based methods instead")
}

// QueryTable executes a query that returns rows for a specific table
func (d *Database) QueryTable(ctx context.Context, table string, filters map[string]string) (*Rows, error) {
	url := fmt.Sprintf("%s/rest/v1/%s", d.config.SupabaseURL, table)
	log.Printf("QueryTable - table: %s, filters: %+v", table, filters)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Add query parameters
	q := req.URL.Query()
	for key, value := range filters {
		q.Add(key, value)
	}
	req.URL.RawQuery = q.Encode()

	log.Printf("Final query URL: %s", req.URL.String())

	// Use service role key for system operations to bypass RLS
	if table == "fcm_tokens" || table == "notifications" {
		req.Header.Set("apikey", d.config.SupabaseServiceRoleKey)
		req.Header.Set("Authorization", "Bearer "+d.config.SupabaseServiceRoleKey)
	} else {
		req.Header.Set("apikey", d.config.SupabaseKey)
		req.Header.Set("Authorization", "Bearer "+d.config.SupabaseKey)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query table: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Query failed with status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("query failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	log.Printf("Query response: %s", string(body))
	return &Rows{data: body}, nil
}

// QueryRow executes a query that returns a single row (compatible with sql.Row interface)
func (d *Database) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	// For REST API, we need to convert SQL query to REST API call
	// This is a simplified implementation that assumes simple SELECT queries
	// In a real implementation, you would need a SQL parser to convert to REST API

	// For now, return a row that will error when scanned
	return &sql.Row{}
}

// Insert inserts data into a table
func (d *Database) Insert(ctx context.Context, table string, data interface{}) (*Rows, error) {
	url := fmt.Sprintf("%s/rest/v1/%s", d.config.SupabaseURL, table)
	log.Printf("Insert - table: %s, data: %+v", table, data)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Use service role key for privileged/system tables
	useServiceRole := table == "fcm_tokens" || table == "notifications" ||
		table == "orders" || table == "payments" || table == "shipments" || table == "delivery_slots"
	if useServiceRole {
		req.Header.Set("apikey", d.config.SupabaseServiceRoleKey)
		req.Header.Set("Authorization", "Bearer "+d.config.SupabaseServiceRoleKey)
	} else {
		req.Header.Set("apikey", d.config.SupabaseKey)
		req.Header.Set("Authorization", "Bearer "+d.config.SupabaseKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to insert data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Insert failed with status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("insert failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	return &Rows{data: body}, nil
}

// Upsert inserts or updates data in a table using Supabase upsert functionality
func (d *Database) Upsert(ctx context.Context, table string, data interface{}, conflictColumns []string) (*Rows, error) {
	url := fmt.Sprintf("%s/rest/v1/%s", d.config.SupabaseURL, table)
	log.Printf("Upsert - table: %s, data: %+v, conflict_columns: %v", table, data, conflictColumns)

	payload, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Use service role key for privileged/system tables
	useServiceRole := table == "fcm_tokens" || table == "notifications" ||
		table == "orders" || table == "payments" || table == "shipments" || table == "delivery_slots"
	if useServiceRole {
		req.Header.Set("apikey", d.config.SupabaseServiceRoleKey)
		req.Header.Set("Authorization", "Bearer "+d.config.SupabaseServiceRoleKey)
	} else {
		req.Header.Set("apikey", d.config.SupabaseKey)
		req.Header.Set("Authorization", "Bearer "+d.config.SupabaseKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "resolution=merge-duplicates")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Upsert failed with status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("upsert failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	log.Printf("Upsert response: %s", string(body))
	return &Rows{data: body}, nil
}

// Update updates data in a table
func (d *Database) Update(ctx context.Context, table string, filters map[string]string, data interface{}) (*Rows, error) {
	url := fmt.Sprintf("%s/rest/v1/%s", d.config.SupabaseURL, table)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Add query parameters for filters
	q := req.URL.Query()
	for key, value := range filters {
		q.Add(key, value)
	}
	req.URL.RawQuery = q.Encode()

	// Use service role key for system operations to bypass RLS
	if table == "fcm_tokens" || table == "notifications" {
		req.Header.Set("apikey", d.config.SupabaseServiceRoleKey)
		req.Header.Set("Authorization", "Bearer "+d.config.SupabaseServiceRoleKey)
	} else {
		req.Header.Set("apikey", d.config.SupabaseKey)
		req.Header.Set("Authorization", "Bearer "+d.config.SupabaseKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to update data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("update failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	return &Rows{data: body}, nil
}

// Delete deletes data from a table
func (d *Database) Delete(ctx context.Context, table string, filters map[string]string) error {
	url := fmt.Sprintf("%s/rest/v1/%s", d.config.SupabaseURL, table)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Add query parameters for filters
	q := req.URL.Query()
	for key, value := range filters {
		q.Add(key, value)
	}
	req.URL.RawQuery = q.Encode()

	// Use service role key for system operations to bypass RLS
	if table == "fcm_tokens" || table == "notifications" {
		req.Header.Set("apikey", d.config.SupabaseServiceRoleKey)
		req.Header.Set("Authorization", "Bearer "+d.config.SupabaseServiceRoleKey)
	} else {
		req.Header.Set("apikey", d.config.SupabaseKey)
		req.Header.Set("Authorization", "Bearer "+d.config.SupabaseKey)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete data: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Close closes the database connection (no-op for REST API)
func (d *Database) Close() error {
	return nil
}

// Ping checks if the database is accessible
func (d *Database) Ping(ctx context.Context) error {
	return d.PingContext(ctx)
}

// BeginTx starts a new transaction (not supported in REST API, returns error)
func (d *Database) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return nil, fmt.Errorf("transactions not supported in REST API mode")
}

// HealthCheck performs a health check on the database
func (d *Database) HealthCheck(ctx context.Context) error {
	return d.Ping(ctx)
}

// GetStats returns database statistics (simplified for REST API)
func (d *Database) GetStats() interface{} {
	return map[string]interface{}{
		"type": "supabase_rest_api",
		"url":  d.config.SupabaseURL,
	}
}

// Result represents the result of an Exec operation
type Result struct{}

func (r *Result) LastInsertId() (int64, error) {
	return 0, nil
}

func (r *Result) RowsAffected() (int64, error) {
	return 0, nil
}

// Rows represents query results
type Rows struct {
	data []byte
}

func (r *Rows) Scan(dest ...interface{}) error {
	return json.Unmarshal(r.data, dest[0])
}

func (r *Rows) Next() bool {
	// Simplified implementation
	return false
}

func (r *Rows) Close() error {
	return nil
}

func (r *Rows) Data() []byte {
	return r.data
}
