package cache

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	// For use with functions that take an expiration time.
	NoExpiration time.Duration = -1
	// Default expiration time is 5 minutes
	DefaultExpiration time.Duration = time.Minute * 5
)

var (
	ErrNotFound  = errors.New("not found")
	ErrEmptyKey  = errors.New("empty key")
	ErrExpired   = errors.New("expired")
	ErrInvalidOp = errors.New("invliad operation")
)

type Item struct {
	Object     interface{}
	Expiration time.Time
}

type ItemMap map[string]*Item

// 1.If the key not found in cache, return an emtpy time and true
// 2.If the key still in cache but the object is expired, return expration time and true
// 3.The object still valid, return object's expration time and false
func (c *Cache) Expired(key string) (time.Time, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	i := c.hash(key)

	c.rwMu[i].RLock()
	defer c.rwMu[i].RUnlock()

	v, found := c.items[i][key]
	if !found {
		return time.Time{}, true
	}
	return v.Expiration, time.Now().After(v.Expiration)
}

type Cache struct {
	*cache
}

type Options struct {
	// Expiration time: used to calculate the item expire time
	// object expire time: time.Now() + Options.Expiration
	Expiration time.Duration
	// Background cleanup interval
	CleanupInterval time.Duration
	// Optional hash function for cache partition
	// Hash function for cache partition
	// The return should be range [0, 128)
	// Default hash function using last byte as the partition key
	// func defaultHash(s string) uint8 {
	// 	return s[len(s)-1] & 0x7F
	// }
	Hash HashFunc
	// Optional function that is called when an object is removed from the cache
	Evicted func(string, interface{})
	// Whether extend the expiration time on a Get operation
	RenewExpirationOnGet bool
}

type HashFunc func(s string) byte

type cache struct {
	mu              sync.RWMutex
	expiration      time.Duration
	cleanupInterval time.Duration
	rwMu            [128]sync.RWMutex
	items           [128]ItemMap
	onEvicted       func(string, interface{})
	hash            HashFunc
	renewOnGet      bool
}

// Default using last byte as the partition key
func defaultHash(s string) uint8 {
	return s[len(s)-1] & 0x7F
}

// Add a new object to cache
// If the key exist, will update the value, old value will be free
func (c *Cache) Set(key string, object interface{}) {
	if key == "" {
		panic("empty key")
	}

	item := &Item{
		Object:     object,
		Expiration: time.Time{},
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.expiration != NoExpiration {
		item.Expiration = time.Now().Add(c.expiration)
	}
	i := c.hash(key)
	c.rwMu[i].Lock()
	c.items[i][key] = item
	c.rwMu[i].Unlock()
}

// Put an object into cache with a specific expiration time
func (c *Cache) SetWithExpiration(key string, object interface{}, expiration time.Duration) error {
	if key == "" {
		return ErrEmptyKey
	}

	item := &Item{
		Object:     object,
		Expiration: time.Now().Add(expiration),
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	i := c.hash(key)
	c.rwMu[i].Lock()
	c.items[i][key] = item
	c.rwMu[i].Unlock()

	return nil
}

// Repalce an object
// If not found, will return err ErrNotFound, will not add the obejct into cache
func (c *Cache) Replace(key string, object interface{}) error {
	if key == "" {
		return ErrEmptyKey
	}

	item := &Item{
		Object:     object,
		Expiration: time.Time{},
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.expiration != NoExpiration {
		item.Expiration = time.Now().Add(c.expiration)
	}

	i := c.hash(key)
	c.rwMu[i].RLock()
	if _, found := c.items[i][key]; !found {
		c.rwMu[i].RUnlock()
		return ErrNotFound
	}

	c.rwMu[i].RUnlock()
	c.rwMu[i].Lock()
	c.items[i][key] = item
	c.rwMu[i].Unlock()

	return nil
}

// Get an object from the cache
// If option RenewExpirationOnGet is enable, may update the object expiration time
func (c *Cache) Get(key string) (interface{}, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	i := c.hash(key)
	c.rwMu[i].RLock()
	if v, found := c.items[i][key]; found {
		if c.expiration == NoExpiration {
			c.rwMu[i].RUnlock()
			return v.Object, nil
		}

		// Found the object, but expired
		if time.Now().After(v.Expiration) {
			c.rwMu[i].RUnlock()
			return nil, ErrExpired
		}

		// Directly return if no need to renew the expiration on read
		if !c.renewOnGet {
			c.rwMu[i].RUnlock()
			return v.Object, nil
		}

		newExpiration := time.Now().Add(c.expiration)
		// Only update expiration time when new expiration after the current expiration
		if newExpiration.After(v.Expiration) {
			c.rwMu[i].RUnlock()
			c.rwMu[i].Lock()
			if _, ok := c.items[i][key]; ok {
				c.items[i][key].Expiration = newExpiration
			}
			c.rwMu[i].Unlock()
			return v.Object, nil
		}
		c.rwMu[i].RUnlock()
		return v.Object, nil
	}
	c.rwMu[i].RUnlock()
	return nil, ErrNotFound
}

// Extend the expiration time with a specific time
// it only works when RenewExpirationOnGet is enabled
func (c *Cache) GetWithExpiration(key string) (interface{}, time.Time, error) {
	if key == "" {
		return nil, time.Time{}, ErrEmptyKey
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	i := c.hash(key)
	c.rwMu[i].RLock()
	if v, found := c.items[i][key]; found {
		if c.expiration == NoExpiration {
			return v.Object, time.Time{}, nil
		}

		if !c.renewOnGet {
			c.rwMu[i].RUnlock()
			return v.Object, v.Expiration, nil
		}

		newExpiration := time.Now().Add(c.expiration)
		// Only update expiration time when new expiration after the current expiration
		if newExpiration.After(v.Expiration) {
			c.rwMu[i].RUnlock()
			c.rwMu[i].Lock()
			if _, ok := c.items[i][key]; ok {
				c.items[i][key].Expiration = newExpiration
			}
			c.rwMu[i].Unlock()
			return v.Object, v.Expiration, nil
		}
	}

	c.rwMu[i].RUnlock()
	return nil, time.Time{}, ErrNotFound
}

// Manually delete objects from the cache, Does nothing if the object does not exist
func (c *Cache) Delete(keys ...string) {
	if len(keys) == 0 {
		return
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, k := range keys {
		if k == "" {
			continue
		}
		i := c.hash(k)
		c.rwMu[i].RLock()
		if v, found := c.items[i][k]; found {
			c.rwMu[i].RUnlock()
			c.rwMu[i].Lock()
			delete(c.items[i], k)
			c.rwMu[i].Unlock()
			if c.onEvicted != nil {
				c.onEvicted(k, v.Object)
			}
		} else {
			c.rwMu[i].RUnlock()
		}
	}
}

// Enable the option RenewExpirationOnGet]
// Get() will extend the object exipration after EnableRenewOnGet()
func (c *Cache) EnableRenewOnGet() {
	c.mu.Lock()
	c.renewOnGet = true
	c.mu.Unlock()
}

// Disable the option RenewExpirationOnGet
// Get() dosen't extend the object exipration time after DisableRenewOnGet()
func (c *Cache) DisableRenewOnGet() {
	c.mu.Lock()
	c.renewOnGet = false
	c.mu.Unlock()
}

// Set the cache expration time which used to calculate object expiration time
// The value only works for new objects,
// if RenewExpirationOnGet is enabled, Get an object will re-calculate the expiration time with the new Expiration
func (c *Cache) SetExpiration(expiration time.Duration) {
	c.mu.Lock()
	c.expiration = expiration
	c.mu.Unlock()
}

func (c *Cache) GetExpiration() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.expiration
}

// Renew the expiration time for the given key
// 1. If the key is not found, return ErrNotFound
// 2. If the key still exist, will renew the expiration time
func (c *Cache) Touch(key string) (interface{}, error) {
	if key == "" {
		return nil, ErrEmptyKey
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	i := c.hash(key)
	c.rwMu[i].RLock()
	v, found := c.items[i][key]
	if !found {
		c.rwMu[i].RUnlock()
		return nil, ErrNotFound
	}

	newExpiration := time.Now().Add(c.expiration)
	if c.expiration != NoExpiration && newExpiration.After(v.Expiration) {
		c.rwMu[i].RUnlock()
		c.rwMu[i].Lock()
		// Double check to make sure the objct exist
		if _, ok := c.items[i][key]; ok {
			c.items[i][key].Expiration = newExpiration
		}
		c.rwMu[i].Unlock()
		return v.Object, nil
	}

	c.rwMu[i].RUnlock()
	return v.Object, nil
}

// Return all item key in the cache, including the expired items
// GetAll don't update expiration time even RenewExpirationOnGet is enabled
func (c *Cache) GetAllKey() []string {
	keys := make([]string, 0, 128)

	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := 0; i < 128; i++ {
		c.rwMu[i].RLock()
		for k := range c.items[i] {
			keys = append(keys, k)
		}
		c.rwMu[i].RUnlock()
	}
	return keys
}

// Get all valid object in the cache, return the Keys
// Don't update expiration time even RenewExpirationOnGet is enabled
func (c *Cache) GetAllValidKey() []string {
	c.mu.RLock()
	if c.expiration == NoExpiration {
		c.mu.RUnlock()
		return c.GetAllKey()
	}
	defer c.mu.RUnlock()

	keys := make([]string, 0, 128)

	for i := 0; i < 128; i++ {
		c.rwMu[i].RLock()
		for k, v := range c.items[i] {
			if time.Now().Before(v.Expiration) {
				keys = append(keys, k)
			}
		}
		c.rwMu[i].RUnlock()
	}
	return keys
}

// Return all item key in the cache, including the expired items
// Don't update expiration time even RenewExpirationOnGet is enabled
func (c *Cache) GetAllObject() map[string]interface{} {
	items := make(map[string]interface{})

	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := 0; i < 128; i++ {
		c.rwMu[i].RLock()
		for k, v := range c.items[i] {
			items[k] = v.Object
		}
		c.rwMu[i].RUnlock()
	}
	return items
}

// Get all valid object in the cache, return the object
// Don't update expiration time even RenewExpirationOnGet is enabled
func (c *Cache) GetAllValidObject() map[string]interface{} {
	c.mu.RLock()
	if c.expiration == NoExpiration {
		c.mu.RUnlock()
		return c.GetAllObject()
	}
	defer c.mu.RUnlock()

	items := make(map[string]interface{})

	for i := 0; i < 128; i++ {
		c.rwMu[i].RLock()
		for k, v := range c.items[i] {
			if time.Now().Before(v.Expiration) {
				items[k] = v.Object
			}
		}
		c.rwMu[i].RUnlock()
	}
	return items
}

func backgroundCleanup(ctx context.Context, c *Cache, interval time.Duration) {
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			c.DeleteExpired(time.Now())
		case <-ctx.Done():
			ticker.Stop()
			return
		}
	}
}

// Delete all expired objects from the cache.
func (c *Cache) DeleteExpired(t time.Time) {
	evictedItems := make([]string, 0, 128)
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := 0; i < 128; i++ {
		c.rwMu[i].RLock()
		for k, v := range c.items[i] {
			if t.After(v.Expiration) {
				evictedItems = append(evictedItems, k)
			}
		}
		c.rwMu[i].RUnlock()
	}

	for _, k := range evictedItems {
		i := c.hash(k)
		c.rwMu[i].Lock()
		v, found := c.items[i][k]
		// Double check
		if found && t.After(v.Expiration) {
			delete(c.items[i], k)
			c.rwMu[i].Unlock()
			if c.onEvicted != nil {
				c.onEvicted(k, v.Object)
			}
		} else {
			c.rwMu[i].Unlock()
		}
	}
}

func (c *Cache) Evicted(onEvicted func(string, interface{})) {
	c.mu.Lock()
	c.onEvicted = onEvicted
	c.mu.Unlock()
}

// Create a Cache
func New(ctx context.Context, opt Options) *Cache {
	c := &cache{
		hash:            defaultHash,
		expiration:      DefaultExpiration,
		cleanupInterval: opt.CleanupInterval,
		onEvicted:       opt.Evicted,
		renewOnGet:      opt.RenewExpirationOnGet,
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for i := 0; i < 128; i++ {
		c.items[i] = make(ItemMap)
	}

	if opt.Expiration > 0 {
		c.expiration = opt.Expiration
	} else {
		c.expiration = NoExpiration
	}

	if opt.Hash != nil {
		c.hash = opt.Hash
	}

	if opt.CleanupInterval > 0 {
		go backgroundCleanup(ctx, &Cache{c}, opt.CleanupInterval)
	}
	return &Cache{c}
}

// Clear the cache, all objects in cache will be deleted
func (c *Cache) Clean() {
	c.cleanup(false)
}

// Clear the cache, compare to function Clear(), Flush() will call onEvicted for the object in cache
func (c *Cache) Flush() {
	c.cleanup(true)
}

func (c *cache) cleanup(evict bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := 0; i < 128; i++ {
		if evict && c.onEvicted != nil {
			for k, v := range c.items[i] {
				c.onEvicted(k, v.Object)
			}
		}
		c.items[i] = make(ItemMap)
	}
}
