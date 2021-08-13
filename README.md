# go-concurrent-cache

Provide a a high perforamnce, thread safety memory cache, some improvement on the go-cache,

```go
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cache := cache.New(ctx, cache.Options{
		Expiration:           time.Second * 1,
		CleanupInterval:      time.Second * 2,
		RenewExpirationOnGet: false,
	})

	cache.Set("key1", 100)
	cache.SetWithExpiration("key2", 200, time.Second*10)

	o1, err := cache.Get("key1")
	if err != nil {
		fmt.Println(err)
	}

	objects := cache.GetAllObject()
}
```