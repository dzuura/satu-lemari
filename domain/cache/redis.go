package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/dzuura/satu-lemari/domain/config"
)

// RedisCache represents Redis cache client
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache client
func NewRedisCache(cfg *config.Config) (*RedisCache, error) {
	// Parse Redis URL
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %v", err)
	}

	// Set password if provided
	if cfg.RedisPassword != "" {
		opt.Password = cfg.RedisPassword
	}

	// Set database if provided
	if cfg.RedisDB != 0 {
		opt.DB = cfg.RedisDB
	}

	// Create Redis client
	client := redis.NewClient(opt)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %v", err)
	}

	log.Println("Redis cache connection established successfully")

	return &RedisCache{client: client}, nil
}

// Close closes the Redis connection
func (r *RedisCache) Close() error {
	return r.client.Close()
}

// Set sets a key-value pair with expiration
func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %v", err)
	}

	return r.client.Set(ctx, key, jsonValue, expiration).Err()
}

// Get retrieves a value by key
func (r *RedisCache) Get(ctx context.Context, key string, dest interface{}) error {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("key not found: %s", key)
		}
		return fmt.Errorf("failed to get key: %v", err)
	}

	return json.Unmarshal([]byte(val), dest)
}

// GetString retrieves a string value by key
func (r *RedisCache) GetString(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", fmt.Errorf("key not found: %s", key)
		}
		return "", fmt.Errorf("failed to get key: %v", err)
	}

	return val, nil
}

// Delete deletes a key
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// DeleteMultiple deletes multiple keys
func (r *RedisCache) DeleteMultiple(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

// Exists checks if a key exists
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check key existence: %v", err)
	}

	return result > 0, nil
}

// Expire sets expiration for a key
func (r *RedisCache) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return r.client.Expire(ctx, key, expiration).Err()
}

// TTL gets time to live for a key
func (r *RedisCache) TTL(ctx context.Context, key string) (time.Duration, error) {
	return r.client.TTL(ctx, key).Result()
}

// Increment increments a counter
func (r *RedisCache) Increment(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, key).Result()
}

// IncrementBy increments a counter by a specific value
func (r *RedisCache) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	return r.client.IncrBy(ctx, key, value).Result()
}

// Decrement decrements a counter
func (r *RedisCache) Decrement(ctx context.Context, key string) (int64, error) {
	return r.client.Decr(ctx, key).Result()
}

// DecrementBy decrements a counter by a specific value
func (r *RedisCache) DecrementBy(ctx context.Context, key string, value int64) (int64, error) {
	return r.client.DecrBy(ctx, key, value).Result()
}

// SetNX sets a key-value pair only if the key doesn't exist
func (r *RedisCache) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("failed to marshal value: %v", err)
	}

	return r.client.SetNX(ctx, key, jsonValue, expiration).Result()
}

// HSet sets a field in a hash
func (r *RedisCache) HSet(ctx context.Context, key string, field string, value interface{}) error {
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %v", err)
	}

	return r.client.HSet(ctx, key, field, jsonValue).Err()
}

// HGet gets a field from a hash
func (r *RedisCache) HGet(ctx context.Context, key string, field string, dest interface{}) error {
	val, err := r.client.HGet(ctx, key, field).Result()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("field not found: %s", field)
		}
		return fmt.Errorf("failed to get field: %v", err)
	}

	return json.Unmarshal([]byte(val), dest)
}

// HGetAll gets all fields from a hash
func (r *RedisCache) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, key).Result()
}

// HDel deletes fields from a hash
func (r *RedisCache) HDel(ctx context.Context, key string, fields ...string) error {
	return r.client.HDel(ctx, key, fields...).Err()
}

// LPush pushes values to the left of a list
func (r *RedisCache) LPush(ctx context.Context, key string, values ...interface{}) error {
	jsonValues := make([]interface{}, len(values))
	for i, value := range values {
		jsonValue, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal value: %v", err)
		}
		jsonValues[i] = jsonValue
	}

	return r.client.LPush(ctx, key, jsonValues...).Err()
}

// RPop pops a value from the right of a list
func (r *RedisCache) RPop(ctx context.Context, key string, dest interface{}) error {
	val, err := r.client.RPop(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return fmt.Errorf("list is empty: %s", key)
		}
		return fmt.Errorf("failed to pop from list: %v", err)
	}

	return json.Unmarshal([]byte(val), dest)
}

// LLen gets the length of a list
func (r *RedisCache) LLen(ctx context.Context, key string) (int64, error) {
	return r.client.LLen(ctx, key).Result()
}

// SAdd adds members to a set
func (r *RedisCache) SAdd(ctx context.Context, key string, members ...interface{}) error {
	jsonMembers := make([]interface{}, len(members))
	for i, member := range members {
		jsonMember, err := json.Marshal(member)
		if err != nil {
			return fmt.Errorf("failed to marshal member: %v", err)
		}
		jsonMembers[i] = jsonMember
	}

	return r.client.SAdd(ctx, key, jsonMembers...).Err()
}

// SRem removes members from a set
func (r *RedisCache) SRem(ctx context.Context, key string, members ...interface{}) error {
	jsonMembers := make([]interface{}, len(members))
	for i, member := range members {
		jsonMember, err := json.Marshal(member)
		if err != nil {
			return fmt.Errorf("failed to marshal member: %v", err)
		}
		jsonMembers[i] = jsonMember
	}

	return r.client.SRem(ctx, key, jsonMembers...).Err()
}

// SMembers gets all members of a set
func (r *RedisCache) SMembers(ctx context.Context, key string) ([]string, error) {
	return r.client.SMembers(ctx, key).Result()
}

// SIsMember checks if a member exists in a set
func (r *RedisCache) SIsMember(ctx context.Context, key string, member interface{}) (bool, error) {
	jsonMember, err := json.Marshal(member)
	if err != nil {
		return false, fmt.Errorf("failed to marshal member: %v", err)
	}

	return r.client.SIsMember(ctx, key, jsonMember).Result()
}

// FlushDB flushes the current database
func (r *RedisCache) FlushDB(ctx context.Context) error {
	return r.client.FlushDB(ctx).Err()
}

// FlushAll flushes all databases
func (r *RedisCache) FlushAll(ctx context.Context) error {
	return r.client.FlushAll(ctx).Err()
}

// Ping pings the Redis server
func (r *RedisCache) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// GetStats returns Redis statistics
func (r *RedisCache) GetStats() *redis.PoolStats {
	return r.client.PoolStats()
}

// GenerateKey generates a cache key with prefix
func (r *RedisCache) GenerateKey(prefix string, parts ...string) string {
	key := prefix
	for _, part := range parts {
		key += ":" + part
	}
	return key
}

// Cache keys constants
const (
	KeyUserProfile     = "user:profile"
	KeyUserStats       = "user:stats"
	KeyItemDetails     = "item:details"
	KeyCategoryList    = "category:list"
	KeyRequestList     = "request:list"
	KeyNotification    = "notification"
	KeyRateLimit       = "ratelimit"
	KeySession         = "session"
	KeyRecommendation  = "recommendation"
	KeySearchResult    = "search:result"
) 