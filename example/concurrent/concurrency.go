package main

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	cache "github.com/huanggangfeng/go-concurrent-cache"
)

var (
	c  *cache.Cache
	wg sync.WaitGroup

	putCount     int64 = 0
	deleteCount  int64 = 0
	getCount     int64 = 0
	cacheHit     int64 = 0
	reclaimCount int64 = 0
	cacheMissed  int64 = 0
)

type TestObject struct {
	ID    int64
	item1 int64
	item2 int64
	item3 int64
	item4 string
}

var testObjectPool = sync.Pool{
	New: func() interface{} {
		return &TestObject{}
	},
}

// User-defined evict funtion to reclaim the object
func onEvicted(key string, obj interface{}) {
	testObjectPool.Put(obj)
	atomic.AddInt64(&reclaimCount, 1)
}

func main() {
	rand.Seed(time.Now().UnixNano())
	ctx, cancel := context.WithCancel(context.Background())

	c = cache.New(ctx, cache.Options{
		Expiration:           time.Second * 1,
		CleanupInterval:      time.Second * 5,
		RenewExpirationOnGet: false,
		Evicted:              onEvicted,
	})

	stopTime := time.Now().Add(time.Second * 20)
	wg.Add(2)
	go PutObject(stopTime)
	go DeleteObject(stopTime)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go GetObject(stopTime)
	}

	wg.Wait()
	c.Flush()
	cancel()
	time.Sleep(time.Second * 1)
	Summary()
}

func Summary() {
	fmt.Println("Summary of cache operation:")
	fmt.Println("Put", putCount)
	fmt.Printf("Get %d, Hit %d, Miss: %d\n", getCount, cacheHit, cacheMissed)
	fmt.Println("Delete", deleteCount)
	fmt.Println("Reclaim", reclaimCount)
}

func PutObject(stop time.Time) {
	for time.Now().Before(stop) {
		c.Set(strconv.FormatInt(putCount, 10), newObject(putCount))
		putCount++
		time.Sleep(time.Millisecond * 20)
	}
	wg.Done()
}

func GetObject(stop time.Time) {
	for time.Now().Before(stop) {
		i := rand.Int63n(20)
		o, err := c.Get(strconv.FormatInt(i, 10))
		if err == nil {
			if !validateObject(o, i) {
				panic("data corrupted")
			}
			atomic.AddInt64(&cacheHit, 1)
		} else {
			atomic.AddInt64(&cacheMissed, 1)
		}
		atomic.AddInt64(&getCount, 1)
		time.Sleep(time.Microsecond * 10)
	}
	wg.Done()
}

func DeleteObject(stop time.Time) {
	for time.Now().Before(stop) {
		key := strconv.FormatInt(deleteCount*2, 10)
		_, err := c.Get(key)
		if err == nil {
			c.Delete(key)
			deleteCount++
			if _, err := c.Get(key); err != cache.ErrNotFound {
				panic("Found an object that has been deleted")
			}
		}
		time.Sleep(time.Millisecond * 2)
	}
	wg.Done()
}

func newObject(i int64) *TestObject {
	obj := testObjectPool.Get().(*TestObject)
	obj.ID = i
	obj.item1 = i + 1
	obj.item2 = i + 2
	obj.item3 = i + 3
	obj.item4 = strconv.FormatInt(i+4, 10)
	return obj
}

func validateObject(object interface{}, i int64) bool {
	o := object.(*TestObject)
	return o.ID == i && o.item1 == i+1 && o.item2 == i+2 && o.item3 == i+3 && o.item4 == strconv.FormatInt(i+4, 10)
}
