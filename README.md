# srcpool   [![GoDoc](https://godoc.org/github.com/tsuna/gohbase?status.png)](https://godoc.org/github.com/henrylee2cn/srcpool)

Srcpool is a high availability / high concurrent resource pool, which is similar to database/sql's db pool.

```
type Pool interface {
    // Name returns the name.
    Name() string
    // Get returns a object in Resource type.
    Get() (Resource, error)
    // GetContext returns a object in Resource type, support context cancellation.
    GetContext(context.Context) (Resource, error)
    // Put gives a resource back to the Pool.
    // If error is not nil, close the avatar.
    Put(Resource, error)
    // SetMaxLifetime sets the maximum amount of time a resource may be reused.
    //
    // Expired resource may be closed lazily before reuse.
    //
    // If d <= 0, resource are reused forever.
    SetMaxLifetime(d time.Duration)
    // SetMaxIdle sets the maximum number of resources in the idle
    // resource pool.
    //
    // If SetMaxIdle is greater than 0 but less than the new MaxIdle
    // then the new MaxIdle will be reduced to match the SetMaxIdle limit
    //
    // If n <= 0, no idle resources are retained.
    SetMaxIdle(n int)
    // SetMaxOpen sets the maximum number of open resources.
    //
    // If MaxIdle is greater than 0 and the new MaxOpen is less than
    // MaxIdle, then MaxIdle will be reduced to match the new
    // MaxOpen limit
    //
    // If n <= 0, then there is no limit on the number of open resources.
    // The default is 0 (unlimited).
    SetMaxOpen(n int)
    // Close closes the Pool, releasing any open resources.
    //
    // It is rare to close a Pool, as the Pool handle is meant to be
    // long-lived and shared between many goroutines.
    Close() error
    // Stats returns resource statistics.
    Stats() PoolStats
}
```