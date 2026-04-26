package playback

import (
	"sync"
	"time"
)

// sfGroup 仿 singleflight.Group，避免引入外部依赖
type sfGroup[V any] struct {
	mu    sync.Mutex
	calls map[string]*sfCall[V]
}

type sfCall[V any] struct {
	done chan struct{}
	val  V
	err  error
}

func (g *sfGroup[V]) Do(key string, fn func() (V, error)) (V, error) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = make(map[string]*sfCall[V])
	}
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		<-c.done
		return c.val, c.err
	}
	c := &sfCall[V]{done: make(chan struct{})}
	g.calls[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	close(c.done)

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()

	return c.val, c.err
}

// sfCache 带 TTL 和 single-flight 合并的进程内缓存
type sfCache[K comparable, V any] struct {
	mu    sync.RWMutex
	items map[K]V
	ts    map[K]time.Time
	ttl   time.Duration
	group sfGroup[V]
}

func newSFCache[K comparable, V any](ttl time.Duration) *sfCache[K, V] {
	return &sfCache[K, V]{
		items: make(map[K]V),
		ts:    make(map[K]time.Time),
		ttl:   ttl,
	}
}

// Get 返回缓存中的值。缓存未命中或已过期时，调用 fn 填充；并发请求会合并为一次 fn 调用。
func (c *sfCache[K, V]) Get(key K, keyStr string, fn func() (V, error)) (V, error) {
	c.mu.RLock()
	if v, ok := c.items[key]; ok && time.Since(c.ts[key]) < c.ttl {
		c.mu.RUnlock()
		return v, nil
	}
	c.mu.RUnlock()

	return c.group.Do(keyStr, func() (V, error) {
		// 再次检查（锁内），避免 fn 调用后的重复写入
		c.mu.RLock()
		if v, ok := c.items[key]; ok && time.Since(c.ts[key]) < c.ttl {
			c.mu.RUnlock()
			return v, nil
		}
		c.mu.RUnlock()

		val, err := fn()
		if err != nil {
			return val, err
		}
		c.mu.Lock()
		c.items[key] = val
		c.ts[key] = time.Now()
		c.mu.Unlock()
		return val, nil
	})
}
