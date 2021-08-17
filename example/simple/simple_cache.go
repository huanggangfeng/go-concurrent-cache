package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	cache "github.com/huanggangfeng/go-concurrent-cache"
)

type testObject struct {
	ID    string
	Value int64
}

func (t testObject) String() string {
	return t.ID + ":" + strconv.FormatInt(t.Value, 10)
}

var (
	key1    = "key1"
	key2    = "key2"
	object1 = testObject{ID: "test1", Value: 66}
	object2 = testObject{ID: "test2", Value: 88}
)

// User-defined evict funtion to reclaim the object
func onEvicted(s string, obj interface{}) {
	fmt.Println("evict:", s)
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := cache.New(ctx, cache.Options{
		Expiration:           time.Second * 1,
		CleanupInterval:      time.Second * 2,
		RenewExpirationOnGet: true,
		Evicted:              onEvicted,
	})

	// Put object1 to cache, using global expiration
	c.Set(key1, object1)

	// Put object2 to cache with a specific expiration
	c.SetWithExpiration(key2, object2, time.Second*10)

	o1, err := c.Get(key1)
	if err != nil {
		panic(err)
	}
	fmt.Println("Get:", o1.(testObject))

	o2, err := c.Get(key2)
	if err != nil {
		panic(err)
	}
	fmt.Println("Get:", o2.(testObject))

	// Wait 5 seconds: object1 will be cleaned from cache
	time.Sleep(time.Second * 5)

	objects := c.GetAllObject()
	fmt.Println("Objects in cache:")
	for k, v := range objects {
		fmt.Println(k, v)
	}

	// Clean all cache
	c.Flush()
	time.Sleep(time.Second)
}
