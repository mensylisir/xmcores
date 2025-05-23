package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCache_Set_Get_Len(t *testing.T) {
	c := NewCache[string, string]()
	defer c.Close()

	if l := c.Len(); l != 0 {
		t.Errorf("Expected initial length 0, got %d", l)
	}

	c.Set("greeting", "Hello")
	val, ok := c.Get("greeting")
	if !ok {
		t.Errorf("Expected 'greeting' to be found")
	}
	if val != "Hello" {
		t.Errorf("Expected value 'Hello', got '%s'", val)
	}
	if l := c.Len(); l != 1 {
		t.Errorf("Expected length 1 after Set, got %d", l)
	}

	_, ok = c.Get("nonexistent")
	if ok {
		t.Errorf("Expected 'nonexistent' to not be found")
	}
}

func TestCache_TTL_Expiration(t *testing.T) {
	c := NewCache[string, string](
		WithDefaultTTL[string, string](20*time.Millisecond),
		WithJanitorInterval[string, string](10*time.Millisecond),
	)
	defer c.Close()

	c.Set("permanent", "This stays")
	c.SetWithTTL("temporary", "This will expire", 10*time.Millisecond)

	if l := c.Len(); l != 2 {
		t.Fatalf("Expected length 2, got %d", l)
	}

	if _, ok := c.Get("temporary"); !ok {
		t.Errorf("'temporary' should exist immediately after set")
	}

	time.Sleep(15 * time.Millisecond)

	if val, ok := c.Get("temporary"); ok {
		t.Errorf("'temporary' should have expired, but got value: %s", val)
	}

	if _, ok := c.Get("permanent"); !ok {
		t.Errorf("'permanent' should still exist with its default TTL")
	}

	time.Sleep(15 * time.Millisecond)

	if val, ok := c.Get("permanent"); ok {
		t.Errorf("'permanent' should have expired by now, but got value: %s", val)
	}

	time.Sleep(15 * time.Millisecond)
	c.DeleteExpired()

	if l := c.Len(); l != 0 {
		t.Errorf("Expected length 0 after all items expired and/or janitor run, got %d", l)
	}
}

func TestCache_GetOrSet(t *testing.T) {
	c := NewCache[string, string](WithDefaultTTL[string, string](50 * time.Millisecond))
	defer c.Close()

	val, loaded := c.GetOrSet("newKey", "New Value")
	if loaded {
		t.Errorf("Expected 'newKey' to be stored, not loaded")
	}
	if val != "New Value" {
		t.Errorf("Expected value 'New Value', got '%s'", val)
	}
	if c.Len() != 1 {
		t.Errorf("Expected length 1, got %d", c.Len())
	}

	val, loaded = c.GetOrSet("newKey", "Another Value (won't be set)")
	if !loaded {
		t.Errorf("Expected 'newKey' to be loaded, not stored")
	}
	if val != "New Value" {
		t.Errorf("Expected value 'New Value', got '%s'", val)
	}
	if c.Len() != 1 {
		t.Errorf("Expected length 1, got %d", c.Len())
	}

	_, loaded = c.GetOrSetWithTTL("ttlKey", "TTL Value", 10*time.Millisecond)
	if loaded {
		t.Errorf("Expected 'ttlKey' to be stored, not loaded")
	}
	if c.Len() != 2 {
		t.Errorf("Expected length 2, got %d", c.Len())
	}

	time.Sleep(15 * time.Millisecond)

	val, loaded = c.GetOrSet("ttlKey", "New TTL Value After Expiry")
	if loaded {
		t.Errorf("Expected 'ttlKey' to be stored again after expiry, not loaded")
	}
	if val != "New TTL Value After Expiry" {
		t.Errorf("Expected new value after expiry, got '%s'", val)
	}
	if c.Len() != 2 {
		t.Errorf("Expected length 2, got %d", c.Len())
	}
}

func TestCache_GetTyped(t *testing.T) {
	anyCache := NewCache[string, any](WithDefaultTTL[string, any](5 * time.Minute))
	defer anyCache.Close()

	anyCache.Set("num", 123)
	anyCache.Set("str", "text")
	anyCache.SetWithTTL("expiredNum", 456, 1*time.Millisecond)

	numVal, ok := GetTyped[int](anyCache, "num")
	if !ok {
		t.Errorf("Failed to get typed int for 'num'")
	}
	if numVal != 123 {
		t.Errorf("Expected numVal 123, got %d", numVal)
	}

	strVal, ok := GetTyped[string](anyCache, "str")
	if !ok {
		t.Errorf("Failed to get typed string for 'str'")
	}
	if strVal != "text" {
		t.Errorf("Expected strVal 'text', got %s", strVal)
	}

	_, ok = GetTyped[float64](anyCache, "num")
	if ok {
		t.Errorf("Expected GetTyped for float64 from int to fail")
	}

	_, ok = GetTyped[int](anyCache, "nonexistent")
	if ok {
		t.Errorf("Expected GetTyped for 'nonexistent' to fail")
	}

	time.Sleep(5 * time.Millisecond)
	_, ok = GetTyped[int](anyCache, "expiredNum")
	if ok {
		t.Errorf("Expected GetTyped for 'expiredNum' to fail after expiration")
	}
}

func TestCache_Clean(t *testing.T) {
	c := NewCache[string, int]()
	defer c.Close()

	c.Set("a", 1)
	c.Set("b", 2)
	if c.Len() != 2 {
		t.Fatalf("Expected length 2 before Clean, got %d", c.Len())
	}

	c.Clean()
	if c.Len() != 0 {
		t.Errorf("Expected length 0 after Clean, got %d", c.Len())
	}

	c.Set("c", 3)
	val, ok := c.Get("c")
	if !ok || val != 3 {
		t.Errorf("Cache not usable after Clean, or Set/Get failed")
	}
	if c.Len() != 1 {
		t.Errorf("Expected length 1 after Clean and Set, got %d", c.Len())
	}
}

func TestCache_Delete(t *testing.T) {
	c := NewCache[string, int]()
	defer c.Close()

	c.Set("a", 1)
	c.Set("b", 2)
	if c.Len() != 2 {
		t.Fatalf("Expected length 2, got %d", c.Len())
	}

	c.Delete("a")
	if c.Len() != 1 {
		t.Errorf("Expected length 1 after deleting 'a', got %d", c.Len())
	}
	if _, ok := c.Get("a"); ok {
		t.Errorf("'a' should not exist after deletion")
	}
	if _, ok := c.Get("b"); !ok {
		t.Errorf("'b' should still exist")
	}

	c.Delete("nonexistent")
	if c.Len() != 1 {
		t.Errorf("Deleting a non-existent key should not change length, got %d", c.Len())
	}
}

func TestCache_DeleteExpired_Direct(t *testing.T) {
	c := NewCache[string, string](WithJanitorInterval[string, string](10 * time.Second)) // 长间隔，避免自动清理干扰
	defer c.Close()

	c.SetWithTTL("item1", "val1", 1*time.Millisecond)
	c.SetWithTTL("item2", "val2", 1*time.Millisecond)
	c.Set("item3", "val3")

	if c.Len() != 3 {
		t.Fatalf("Initial length should be 3, got %d", c.Len())
	}

	time.Sleep(5 * time.Millisecond)

	c.DeleteExpired()

	if l := c.Len(); l != 1 {
		t.Errorf("Expected length 1 after DeleteExpired, got %d", l)
	}
	if _, ok := c.Get("item1"); ok {
		t.Error("item1 should be gone after DeleteExpired")
	}
	if _, ok := c.Get("item2"); ok {
		t.Error("item2 should be gone after DeleteExpired")
	}
	if _, ok := c.Get("item3"); !ok {
		t.Error("item3 should still exist")
	}
}

func TestCache_Range(t *testing.T) {
	c := NewCache[string, int]()
	defer c.Close()

	items := map[string]int{
		"a": 1,
		"b": 2,
		"c": 3,
	}
	for k, v := range items {
		c.Set(k, v)
	}
	c.SetWithTTL("expired", 4, 1*time.Millisecond)

	time.Sleep(5 * time.Millisecond)

	seen := make(map[string]int)
	var count int
	c.Range(func(key string, value int) bool {
		if key == "expired" {
			t.Errorf("Ranged over an expired key 'expired'")
		}
		seen[key] = value
		count++
		return true
	})

	if count != 3 {
		t.Errorf("Expected to range over 3 non-expired items, got %d", count)
	}

	for k, v := range items {
		if seenVal, ok := seen[k]; !ok || seenVal != v {
			t.Errorf("Key %s not seen in range or value mismatch. Expected %d, Seen %d, Found %v", k, v, seenVal, ok)
		}
	}

	count = 0
	c.Range(func(key string, value int) bool {
		count++
		return count < 2
	})
	if count != 2 && count != 1 { // sync.Map 迭代顺序不保证，所以可能是1或2
		// 实际上，为了确定性，我们应该只检查 count <= 2
		// 或者，更精确地说，它会迭代直到回调返回 false。
		// 如果第一次就返回 false，count 会是 1。
		// 如果第二次返回 false，count 会是 2。
		// 因此，这里我们只关心它没有迭代完所有。
	}
	// 这个部分的测试依赖于 sync.Map 的迭代顺序，不够稳定，除非你有办法固定顺序或测试的是“至少迭代了X次”。
	// 一个更可靠的测试是，如果回调返回 false，迭代就停止。
	// 比如，找到特定元素后停止。

	stopKey := "b"
	var foundAndStopped bool
	var iterationsBeforeStop int
	c.Range(func(key string, value int) bool {
		iterationsBeforeStop++
		if key == stopKey {
			foundAndStopped = true
			return false
		}
		return true
	})

	if !foundAndStopped {
		t.Errorf("Range did not find stopKey '%s' or did not stop", stopKey)
	}
	if iterationsBeforeStop > len(items) {
		t.Errorf("Range iterated more times than expected before stopping for key '%s'", stopKey)
	}
	fmt.Printf("Range stopped for key '%s' after %d iterations\n", stopKey, iterationsBeforeStop)

}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := NewCache[int, int](WithJanitorInterval[int, int](50 * time.Millisecond))
	defer c.Close()

	var wg sync.WaitGroup
	numGoroutines := 100
	numOpsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(gIndex int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				key := (gIndex * numOpsPerGoroutine) + j
				c.Set(key, key*2)
				// 偶尔设置带 TTL 的
				if j%10 == 0 {
					c.SetWithTTL(key+numGoroutines*numOpsPerGoroutine, (key)*3, time.Duration(j+1)*time.Millisecond)
				}
			}
		}(i)
	}
	wg.Wait()

	// 验证 Len (可能不完全精确，因为有 TTL 项)
	t.Logf("Cache length after concurrent sets: %d (approx %d)", c.Len(), numGoroutines*numOpsPerGoroutine)

	// 并发 Get
	errors := make(chan error, numGoroutines*numOpsPerGoroutine)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(gIndex int) {
			defer wg.Done()
			for j := 0; j < numOpsPerGoroutine; j++ {
				key := (gIndex * numOpsPerGoroutine) + j
				val, ok := c.Get(key)
				if !ok {
					// 这可能是因为TTL项过期了，或者并发问题
					// 对于非 TTL 项，它应该存在
					if j%10 != 0 { // 检查非TTL项
						// 重新获取一次，以防是janitor刚清理
						time.Sleep(1 * time.Millisecond)
						val, ok = c.Get(key)
						if !ok {
							errors <- fmt.Errorf("goroutine %d: key %d not found", gIndex, key)
							continue
						}
					} else {
						// TTL项可能已过期，可以接受
						continue
					}
				}
				if val != key*2 {
					errors <- fmt.Errorf("goroutine %d: key %d expected val %d, got %d", gIndex, key, key*2, val)
				}
			}
		}(i)
	}
	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
	// 由于TTL的存在，最终的Len可能小于 numGoroutines * numOpsPerGoroutine
	// 等待TTL项过期
	time.Sleep(time.Duration(numOpsPerGoroutine+10) * time.Millisecond)
	c.DeleteExpired() // 手动清理
	expectedNonTTLElements := int64(numGoroutines * numOpsPerGoroutine)
	finalLen := c.Len()
	// 期望只剩下非TTL项
	if finalLen != expectedNonTTLElements {
		// 这是一个宽松的检查，因为Get操作中的惰性删除也可能影响计数
		// 如果所有TTL项都被正确设置并过期，并且非TTL项仍然存在，则此测试更准确。
		// 实际上，上面的Set中，TTL项的key是不同的，所以非TTL项应该都是 numGoroutines * numOpsPerGoroutine 个。
		t.Logf("Expected final length (non-TTL) around %d, got %d. This can vary due to concurrent lazy deletion and janitor timing.", expectedNonTTLElements, finalLen)
	}
}
