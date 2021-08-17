# go-concurrent-cache

Provide a a high perforamnce, thread safety memory cache, some improvement on the go-cache,

```go
func onEvicted(s string, obj interface{}) {
	....
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache := cache.New(ctx, cache.Options{
		Expiration:           time.Second * 1,
		CleanupInterval:      time.Second * 10,
		RenewExpirationOnGet: false,
		Evicted:              onEvicted,
	})

	cache.Set("key1", 100)
	cache.SetWithExpiration("key2", 200, time.Second*10)

	o1, err := cache.Get("key1")
	if err != nil {
		fmt.Println(err)
	}

	objects := cache.GetAllObject()
	cache.Clean()
}
```
