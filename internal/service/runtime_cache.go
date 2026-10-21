package service

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Ferrymania/nook/internal/config"
)

const (
	defaultRuntimeCacheEntryLimit     = 2048
	runtimeCacheEntryOverheadBytes    = 96
	runtimeCacheMapEntryOverheadBytes = 64
	runtimeCacheStringHeaderBytes     = 16
	runtimeCacheEstimateMaxDepth      = 64
)

type RuntimeCacheService struct {
	mu        sync.RWMutex
	memory    map[string]runtimeCacheItem
	obj       map[string]runtimeObjectItem
	limit     int
	maxBytes  int64
	bytesUsed int64
	// seq stamps every insert/access with a strictly increasing number. Two
	// entries written or touched inside the same clock tick carry an identical
	// lastUsed, and Go map iteration order is random, so eviction used to pick an
	// arbitrary "oldest" entry. seq turns recency into a total order.
	seq uint64
}

func (c *RuntimeCacheService) nextSeqLocked() uint64 {
	c.seq++
	return c.seq
}

type runtimeCacheItem struct {
	raw        []byte
	expiresAt  time.Time
	staleUntil time.Time
	lastUsed   time.Time
	seq        uint64
	size       int64
}

// runtimeObjectItem 直存 Go 对象，跳过 JSON 编解码。热点路径（整库行、
// 分组结果）每次请求都要完整反序列化，字节缓存避免了 SQL 却没避免解码；
// 对象缓存命中时零解码零分配。存储的值视为不可变：读取方如需修改必须
// 自行浅拷贝。每个进程独立缓存，和 JSON 缓存共用容量与淘汰预算。
type runtimeObjectItem struct {
	value     any
	expiresAt time.Time
	lastUsed  time.Time
	seq       uint64
	size      int64
}

func NewRuntimeCacheService(cfg *config.Config) *RuntimeCacheService {
	maxBytes := int64(config.DefaultCacheMemoryMaxSizeMB) * 1024 * 1024
	if cfg != nil && cfg.Cache.MemoryMaxSizeMB > 0 {
		maxBytes = int64(cfg.Cache.MemoryMaxSizeMB) * 1024 * 1024
	}
	c := &RuntimeCacheService{
		memory:   map[string]runtimeCacheItem{},
		obj:      map[string]runtimeObjectItem{},
		limit:    defaultRuntimeCacheEntryLimit,
		maxBytes: maxBytes,
	}
	return c
}

func (c *RuntimeCacheService) Enabled() bool {
	return c != nil
}

func (c *RuntimeCacheService) GetJSON(_ context.Context, key string, out any) bool {
	if !c.Enabled() || strings.TrimSpace(key) == "" || out == nil {
		return false
	}
	raw, ok := c.getMemory(c.key(key))
	return ok && json.Unmarshal(raw, out) == nil
}

// GetJSONStale returns a fresh value when available and otherwise a retained
// stale value stored with SetJSONWithStale. The stale flag lets
// callers serve immediately while refreshing in the background.
func (c *RuntimeCacheService) GetJSONStale(_ context.Context, key string, out any) (found bool, stale bool) {
	if !c.Enabled() || strings.TrimSpace(key) == "" || out == nil {
		return false, false
	}
	raw, ok, isStale := c.getMemoryWithStale(c.key(key))
	if !ok {
		return false, false
	}
	return json.Unmarshal(raw, out) == nil, isStale
}

func (c *RuntimeCacheService) SetJSON(_ context.Context, key string, value any, ttl time.Duration) {
	if !c.Enabled() || strings.TrimSpace(key) == "" || value == nil || ttl <= 0 {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	fullKey := c.key(key)
	c.setMemoryOwned(fullKey, raw, ttl)
}

// SetJSONWithStale stores a fresh value for freshTTL and keeps an in-process
// stale copy for staleTTL. Callers may serve it while refreshing in the background.
func (c *RuntimeCacheService) SetJSONWithStale(_ context.Context, key string, value any, freshTTL, staleTTL time.Duration) {
	if !c.Enabled() || strings.TrimSpace(key) == "" || value == nil || freshTTL <= 0 {
		return
	}
	if staleTTL < freshTTL {
		staleTTL = freshTTL
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	fullKey := c.key(key)
	c.setMemoryBytesWithStale(fullKey, raw, freshTTL, staleTTL, true)
}

// GetObject 返回缓存中的对象。返回值不可变：调用方需要修改时必须先自行拷贝。
func (c *RuntimeCacheService) GetObject(key string) (any, bool) {
	if !c.Enabled() || strings.TrimSpace(key) == "" {
		return nil, false
	}
	fullKey := c.key(key)
	now := time.Now()
	c.mu.Lock()
	item, ok := c.obj[fullKey]
	if !ok {
		c.mu.Unlock()
		return nil, false
	}
	if !now.Before(item.expiresAt) {
		c.removeObjectLocked(fullKey)
		c.mu.Unlock()
		return nil, false
	}
	item.lastUsed = now
	item.seq = c.nextSeqLocked()
	c.obj[fullKey] = item
	c.mu.Unlock()
	return item.value, true
}

// SetObject 存入一个此后视为不可变的对象。对象会按估算内存计入进程内缓存
// 总预算；单个对象超过预算时不会进入缓存，避免一次大列表请求再次打爆内存。
func (c *RuntimeCacheService) SetObject(key string, value any, ttl time.Duration) {
	if !c.Enabled() || strings.TrimSpace(key) == "" || value == nil || ttl <= 0 {
		return
	}
	fullKey := c.key(key)
	size := estimateRuntimeCacheObjectSize(fullKey, value)
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if size <= 0 || size > c.maxBytes {
		return
	}
	c.removeObjectLocked(fullKey)
	if !c.makeRoomLocked(now, size) {
		return
	}
	c.obj[fullKey] = runtimeObjectItem{
		value:     value,
		expiresAt: now.Add(ttl),
		lastUsed:  now,
		seq:       c.nextSeqLocked(),
		size:      size,
	}
	c.bytesUsed += size
}

// SetMaxSizeMB 热更新进程内缓存总预算。降低上限时会立即淘汰最久未使用的
// 条目；传 0 或负数时恢复默认预算。
func (c *RuntimeCacheService) SetMaxSizeMB(maxMB int) {
	if c == nil {
		return
	}
	if maxMB <= 0 {
		maxMB = config.DefaultCacheMemoryMaxSizeMB
	}
	maxBytes := int64(maxMB) * 1024 * 1024
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxBytes = maxBytes
	c.evictExpiredLocked(now)
	for c.bytesUsed > c.maxBytes {
		if !c.evictOldestLocked() {
			break
		}
	}
}

func (c *RuntimeCacheService) DeletePrefix(_ context.Context, prefix string) {
	if !c.Enabled() || strings.TrimSpace(prefix) == "" {
		return
	}
	c.deleteMemoryPrefix(c.key(prefix))
}

func (*RuntimeCacheService) key(key string) string {
	return strings.TrimLeft(strings.TrimSpace(key), ":")
}

func (c *RuntimeCacheService) getMemory(key string) ([]byte, bool) {
	raw, ok, stale := c.getMemoryWithStale(key)
	return raw, ok && !stale
}

func (c *RuntimeCacheService) getMemoryWithStale(key string) ([]byte, bool, bool) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.memory[key]
	if !ok {
		return nil, false, false
	}
	staleUntil := item.staleUntil
	if staleUntil.IsZero() {
		staleUntil = item.expiresAt
	}
	if !now.Before(staleUntil) {
		c.removeMemoryLocked(key)
		return nil, false, false
	}
	item.lastUsed = now
	item.seq = c.nextSeqLocked()
	c.memory[key] = item
	return item.raw, true, !now.Before(item.expiresAt)
}

func (c *RuntimeCacheService) setMemory(key string, raw []byte, ttl time.Duration) {
	c.setMemoryBytes(key, raw, ttl, false)
}

func (c *RuntimeCacheService) setMemoryOwned(key string, raw []byte, ttl time.Duration) {
	c.setMemoryBytes(key, raw, ttl, true)
}

func (c *RuntimeCacheService) setMemoryBytes(key string, raw []byte, ttl time.Duration, owned bool) {
	c.setMemoryBytesWithStale(key, raw, ttl, ttl, owned)
}

func (c *RuntimeCacheService) setMemoryBytesWithStale(key string, raw []byte, freshTTL, staleTTL time.Duration, owned bool) {
	if freshTTL <= 0 || len(raw) == 0 {
		return
	}
	if staleTTL < freshTTL {
		staleTTL = freshTTL
	}
	size := int64(len(key)+len(raw)) + runtimeCacheEntryOverheadBytes
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if size <= 0 || size > c.maxBytes {
		return
	}
	c.removeMemoryLocked(key)
	if !c.makeRoomLocked(now, size) {
		return
	}
	if !owned {
		raw = append([]byte(nil), raw...)
	}
	c.memory[key] = runtimeCacheItem{
		raw:        raw,
		expiresAt:  now.Add(freshTTL),
		staleUntil: now.Add(staleTTL),
		lastUsed:   now,
		seq:        c.nextSeqLocked(),
		size:       size,
	}
	c.bytesUsed += size
}

func (c *RuntimeCacheService) makeRoomLocked(now time.Time, incoming int64) bool {
	if incoming <= 0 {
		incoming = 1
	}
	if c.limit <= 0 {
		c.limit = defaultRuntimeCacheEntryLimit
	}
	if c.maxBytes <= 0 {
		c.maxBytes = int64(config.DefaultCacheMemoryMaxSizeMB) * 1024 * 1024
	}
	if incoming > c.maxBytes {
		return false
	}
	c.evictExpiredLocked(now)
	for c.entryCountLocked() >= c.limit || c.bytesUsed+incoming > c.maxBytes {
		if !c.evictOldestLocked() {
			return false
		}
	}
	return true
}

func (c *RuntimeCacheService) entryCountLocked() int {
	return len(c.memory) + len(c.obj)
}

func (c *RuntimeCacheService) evictExpiredLocked(now time.Time) {
	for key, item := range c.memory {
		staleUntil := item.staleUntil
		if staleUntil.IsZero() {
			staleUntil = item.expiresAt
		}
		if !now.Before(staleUntil) {
			c.removeMemoryLocked(key)
		}
	}
	for key, item := range c.obj {
		if !now.Before(item.expiresAt) {
			c.removeObjectLocked(key)
		}
	}
}

// evictOldestLocked removes the least recently used entry across both caches.
//
// Recency is ordered by (lastUsed, seq): entries written within the same clock
// tick share lastUsed, and map iteration order is randomized, so comparing
// timestamps alone let eviction drop a just-written entry instead of the older
// one. seq breaks those ties deterministically.
func (c *RuntimeCacheService) evictOldestLocked() bool {
	var (
		oldestKey  string
		oldestKind byte
		oldestAt   time.Time
		oldestSeq  uint64
	)
	older := func(at time.Time, seq uint64) bool {
		if oldestKind == 0 {
			return true
		}
		if at.Equal(oldestAt) {
			return seq < oldestSeq
		}
		return at.Before(oldestAt)
	}
	for key, item := range c.memory {
		if older(item.lastUsed, item.seq) {
			oldestKey, oldestKind, oldestAt, oldestSeq = key, 'm', item.lastUsed, item.seq
		}
	}
	for key, item := range c.obj {
		if older(item.lastUsed, item.seq) {
			oldestKey, oldestKind, oldestAt, oldestSeq = key, 'o', item.lastUsed, item.seq
		}
	}
	switch oldestKind {
	case 'm':
		c.removeMemoryLocked(oldestKey)
		return true
	case 'o':
		c.removeObjectLocked(oldestKey)
		return true
	default:
		return false
	}
}

func (c *RuntimeCacheService) removeMemoryLocked(key string) {
	item, ok := c.memory[key]
	if !ok {
		return
	}
	delete(c.memory, key)
	c.bytesUsed -= item.size
	if c.bytesUsed < 0 {
		c.bytesUsed = 0
	}
}

func (c *RuntimeCacheService) removeObjectLocked(key string) {
	item, ok := c.obj[key]
	if !ok {
		return
	}
	delete(c.obj, key)
	c.bytesUsed -= item.size
	if c.bytesUsed < 0 {
		c.bytesUsed = 0
	}
}

func (c *RuntimeCacheService) deleteMemoryPrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.memory {
		if strings.HasPrefix(key, prefix) {
			c.removeMemoryLocked(key)
		}
	}
	for key := range c.obj {
		if strings.HasPrefix(key, prefix) {
			c.removeObjectLocked(key)
		}
	}
}

type runtimeCacheVisit struct {
	kind reflect.Kind
	ptr  uintptr
}

func estimateRuntimeCacheObjectSize(key string, value any) int64 {
	seen := make(map[runtimeCacheVisit]struct{})
	size := int64(len(key)) + runtimeCacheEntryOverheadBytes
	size += estimateRuntimeCacheReflectSize(reflect.ValueOf(value), seen, 0)
	return size
}

func estimateRuntimeCacheReflectSize(v reflect.Value, seen map[runtimeCacheVisit]struct{}, depth int) int64 {
	if !v.IsValid() || depth > runtimeCacheEstimateMaxDepth {
		return 0
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return 0
		}
		return estimateRuntimeCacheReflectSize(v.Elem(), seen, depth+1)
	case reflect.Pointer:
		if v.IsNil() || markRuntimeCacheVisit(v, seen) {
			return 0
		}
		return int64(v.Type().Elem().Size()) + estimateRuntimeCacheNestedSize(v.Elem(), seen, depth+1)
	case reflect.String:
		return int64(len(v.String())) + runtimeCacheStringHeaderBytes
	case reflect.Slice:
		if v.IsNil() || markRuntimeCacheVisit(v, seen) {
			return 0
		}
		total := int64(v.Type().Size()) + int64(v.Cap())*int64(v.Type().Elem().Size())
		for i := 0; i < v.Len(); i++ {
			total += estimateRuntimeCacheNestedSize(v.Index(i), seen, depth+1)
		}
		return total
	case reflect.Map:
		if v.IsNil() || markRuntimeCacheVisit(v, seen) {
			return 0
		}
		total := int64(v.Type().Size()) + int64(v.Len())*runtimeCacheMapEntryOverheadBytes
		iter := v.MapRange()
		for iter.Next() {
			total += estimateRuntimeCacheNestedSize(iter.Key(), seen, depth+1)
			total += estimateRuntimeCacheNestedSize(iter.Value(), seen, depth+1)
		}
		return total
	case reflect.Struct:
		total := int64(v.Type().Size())
		for i := 0; i < v.NumField(); i++ {
			total += estimateRuntimeCacheNestedSize(v.Field(i), seen, depth+1)
		}
		return total
	case reflect.Array:
		total := int64(v.Type().Size())
		for i := 0; i < v.Len(); i++ {
			total += estimateRuntimeCacheNestedSize(v.Index(i), seen, depth+1)
		}
		return total
	default:
		return int64(v.Type().Size())
	}
}

func estimateRuntimeCacheNestedSize(v reflect.Value, seen map[runtimeCacheVisit]struct{}, depth int) int64 {
	if !v.IsValid() || depth > runtimeCacheEstimateMaxDepth {
		return 0
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		return estimateRuntimeCacheReflectSize(v, seen, depth+1)
	case reflect.String:
		return int64(len(v.String()))
	case reflect.Slice:
		if v.IsNil() || markRuntimeCacheVisit(v, seen) {
			return 0
		}
		total := int64(v.Cap()) * int64(v.Type().Elem().Size())
		for i := 0; i < v.Len(); i++ {
			total += estimateRuntimeCacheNestedSize(v.Index(i), seen, depth+1)
		}
		return total
	case reflect.Map:
		if v.IsNil() || markRuntimeCacheVisit(v, seen) {
			return 0
		}
		total := int64(v.Len()) * runtimeCacheMapEntryOverheadBytes
		iter := v.MapRange()
		for iter.Next() {
			total += estimateRuntimeCacheNestedSize(iter.Key(), seen, depth+1)
			total += estimateRuntimeCacheNestedSize(iter.Value(), seen, depth+1)
		}
		return total
	case reflect.Struct:
		total := int64(0)
		for i := 0; i < v.NumField(); i++ {
			total += estimateRuntimeCacheNestedSize(v.Field(i), seen, depth+1)
		}
		return total
	case reflect.Array:
		total := int64(0)
		for i := 0; i < v.Len(); i++ {
			total += estimateRuntimeCacheNestedSize(v.Index(i), seen, depth+1)
		}
		return total
	default:
		return 0
	}
}

func markRuntimeCacheVisit(v reflect.Value, seen map[runtimeCacheVisit]struct{}) bool {
	ptr := v.Pointer()
	if ptr == 0 {
		return false
	}
	visit := runtimeCacheVisit{kind: v.Kind(), ptr: ptr}
	if _, ok := seen[visit]; ok {
		return true
	}
	seen[visit] = struct{}{}
	return false
}
