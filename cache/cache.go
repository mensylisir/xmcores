package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

// CacheItem represents an item stored in the cache, including its expiration time.
type cacheItem[V any] struct {
	value     V
	expiresAt int64 // UnixNano timestamp, 0 means no expiration
}

// IsExpired checks if the cache item has expired.
func (item *cacheItem[V]) IsExpired() bool {
	if item.expiresAt == 0 {
		return false // No expiration
	}
	return time.Now().UnixNano() > item.expiresAt
}

// Cache is a thread-safe, generic cache with TTL support.
type Cache[K comparable, V any] struct {
	store         sync.Map
	defaultTTL    time.Duration
	janitorOnce   sync.Once
	janitorTicker *time.Ticker
	stopJanitorCh chan struct{}
	itemCount     atomic.Int64
}

// Option is a functional option type for Cache configuration.
type Option[K comparable, V any] func(*Cache[K, V])

// WithDefaultTTL sets the default Time-To-Live for items in the cache.
// Items set without a specific TTL will use this value.
func WithDefaultTTL[K comparable, V any](ttl time.Duration) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.defaultTTL = ttl
	}
}

// WithJanitorInterval sets the interval at which the janitor cleans up expired items.
// If interval is 0 or negative, the janitor will not run automatically.
// The janitor is started on the first Set operation with a TTL or if defaultTTL is set.
func WithJanitorInterval[K comparable, V any](interval time.Duration) Option[K, V] {
	return func(c *Cache[K, V]) {
		if interval > 0 {
			c.janitorTicker = time.NewTicker(interval)
			c.stopJanitorCh = make(chan struct{})
		}
	}
}

// NewCache creates a new Cache instance with optional configurations.
func NewCache[K comparable, V any](opts ...Option[K, V]) *Cache[K, V] {
	c := &Cache[K, V]{
		// Default janitor interval if not set by options
		janitorTicker: time.NewTicker(5 * time.Minute), // Default, can be overridden
		stopJanitorCh: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(c)
	}

	// If a janitor ticker was configured (either default or by option), start the janitor.
	// The janitor is lazy-started on first relevant Set or explicitly by configuration.
	// We'll ensure it's truly started if needed in SetWithTTL or if defaultTTL is active.
	return c
}

func (c *Cache[K, V]) startJanitor() {
	c.janitorOnce.Do(func() {
		if c.janitorTicker != nil { // Ensure ticker was initialized
			go func() {
				for {
					select {
					case <-c.janitorTicker.C:
						c.DeleteExpired()
					case <-c.stopJanitorCh:
						c.janitorTicker.Stop()
						return
					}
				}
			}()
		}
	})
}

// Set adds or updates an item in the cache with the default TTL (if configured).
// If defaultTTL is 0, the item will not expire unless SetWithTTL is used.
func (c *Cache[K, V]) Set(k K, v V) {
	c.SetWithTTL(k, v, c.defaultTTL)
}

// SetWithTTL adds or updates an item in the cache with a specific TTL.
// If ttl is 0, the item will not expire.
// If ttl is negative, the item is effectively treated as already expired and might not be stored or will be removed quickly.
func (c *Cache[K, V]) SetWithTTL(k K, v V, ttl time.Duration) {
	var expiresAt int64
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl).UnixNano()
		if c.janitorTicker != nil { // Start janitor if we have TTL items and a ticker
			c.startJanitor()
		}
	} else if ttl < 0 { // Negative TTL means expire immediately
		c.Delete(k) // Ensure it's removed if it exists
		return
	}

	item := &cacheItem[V]{
		value:     v,
		expiresAt: expiresAt,
	}

	_, loaded := c.store.LoadOrStore(k, item)
	if !loaded {
		c.itemCount.Add(1)
	} else {
		// If loaded, it means an old item was there. We are replacing it.
		// The count doesn't change, but we need to store the new item.
		c.store.Store(k, item)
	}
}

// Get retrieves an item from the cache.
// It returns the value and true if the item exists and has not expired.
// Otherwise, it returns the zero value for V and false.
func (c *Cache[K, V]) Get(k K) (V, bool) {
	var zeroV V
	loaded, ok := c.store.Load(k)
	if !ok {
		return zeroV, false // Not found
	}

	item, ok := loaded.(*cacheItem[V])
	if !ok {
		// This should not happen if Set is used correctly
		// but handle defensively: remove malformed entry.
		c.store.Delete(k)
		c.itemCount.Add(-1) // Adjust count, though it implies prior miscounting
		return zeroV, false
	}

	if item.IsExpired() {
		c.store.Delete(k) // Lazy deletion
		c.itemCount.Add(-1)
		return zeroV, false // Expired
	}

	return item.value, true
}

// GetOrSet returns the existing value for the key if present and not expired.
// Otherwise, it stores and returns the given value (with default TTL).
// The loaded result is true if the value was loaded, false if stored.
func (c *Cache[K, V]) GetOrSet(k K, v V) (V, bool) {
	return c.GetOrSetWithTTL(k, v, c.defaultTTL)
}

// GetOrSetWithTTL returns the existing value for the key if present and not expired.
// Otherwise, it stores and returns the given value with the specified TTL.
// The loaded result is true if the value was loaded, false if stored.
func (c *Cache[K, V]) GetOrSetWithTTL(k K, v V, ttl time.Duration) (V, bool) {
	// First, try to get
	if existingVal, found := c.Get(k); found {
		return existingVal, true // Loaded
	}

	// Not found or expired, so set it
	c.SetWithTTL(k, v, ttl)
	return v, false // Stored
}

// Delete removes an item from the cache.
func (c *Cache[K, V]) Delete(k K) {
	if _, loaded := c.store.LoadAndDelete(k); loaded {
		c.itemCount.Add(-1)
	}
}

// DeleteExpired manually triggers a cleanup of expired items.
// This is also called periodically by the janitor goroutine if configured.
func (c *Cache[K, V]) DeleteExpired() {
	c.store.Range(func(key, value interface{}) bool {
		k := key.(K) // Assume key is of type K
		item, ok := value.(*cacheItem[V])
		if !ok { // Should not happen
			c.store.Delete(k)
			// Cannot reliably decrement itemCount here as we don't know if it was counted
			return true
		}
		if item.IsExpired() {
			if _, loaded := c.store.LoadAndDelete(k); loaded {
				c.itemCount.Add(-1)
			}
		}
		return true
	})
}

// Range iterates over the cache items, calling f for each key and unexpired value.
// If f returns false, range stops the iteration.
// Note: Iteration order is not guaranteed.
func (c *Cache[K, V]) Range(f func(key K, value V) bool) {
	now := time.Now().UnixNano()
	c.store.Range(func(key, value interface{}) bool {
		k := key.(K)
		item, ok := value.(*cacheItem[V])
		if !ok {
			return true // Skip malformed entries
		}

		// Check for expiration during range, but don't delete here to avoid map modification issues
		// during iteration that `sync.Map.Range` might not like if we `Delete` directly.
		// Let lazy Get or Janitor handle deletion.
		if item.expiresAt != 0 && now > item.expiresAt {
			// Optionally, one could delete here, but it's safer to let Get/Janitor do it.
			// For now, we just don't pass expired items to the callback.
			return true
		}
		return f(k, item.value)
	})
}

// Clean removes all items from the cache.
func (c *Cache[K, V]) Clean() {
	// The most efficient way to clear a sync.Map is to replace it,
	// but we also need to reset the item count.
	c.store = sync.Map{}
	c.itemCount.Store(0) // Reset count
}

// Len returns the current number of items in the cache.
// This count includes items that might be expired but not yet collected by the janitor.
// For a count of only non-expired items, you would need to iterate.
func (c *Cache[K, V]) Len() int64 {
	return c.itemCount.Load()
}

// Close stops the janitor goroutine if it's running.
// It's important to call Close when the cache is no longer needed to free resources.
func (c *Cache[K, V]) Close() {
	if c.janitorTicker != nil { // Check if janitor was ever configured to run
		// Check if stopJanitorCh was initialized (it is if janitorTicker is)
		if c.stopJanitorCh != nil {
			select {
			case c.stopJanitorCh <- struct{}{}: // Signal janitor to stop
			default: // Avoid blocking if channel is unbuffered and janitor already stopped or not started
			}
		}
		// Ticker is stopped inside the janitor goroutine upon receiving from stopJanitorCh
	}
}

// GetMust... type helper methods are less critical with generics,
// as Cache[K, string] already implies string values.
// However, if V is an interface type (e.g., any), they can be useful.
// Let's provide a generic GetTyped for this purpose.

// GetTyped retrieves an item and asserts it to type T.
// Returns the typed value and true if the key exists, the item is not expired,
// and the type assertion is successful.
func GetTyped[T any, K comparable, V any](c *Cache[K, V], k K) (T, bool) {
	var zeroT T
	val, ok := c.Get(k)
	if !ok {
		return zeroT, false
	}
	typedVal, typeOk := any(val).(T) // any(val) is needed if V is a concrete type different from T
	if !typeOk {
		return zeroT, false
	}
	return typedVal, true
}
