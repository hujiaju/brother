package proxy

import "sync/atomic"

type Counter struct {
	OldClientQPS				int64
	OldErrLogTotal				int64
	OldSlowLogTotal				int64

	ClientConns				int64
	ClientQPS				int64
	ErrLogTotal				int64
	SlowLogTotal				int64
}

func (c *Counter) IncrClientConns()  {
	atomic.AddInt64(&c.ClientConns, 1)
}

func (c *Counter) DecrClientConns()  {
	atomic.AddInt64(&c.ClientConns, -1)
}

func (c *Counter) IncrClientQPS()  {
	atomic.AddInt64(&c.ClientQPS, 1)
}

func (c *Counter) DecrClientQPS()  {
	atomic.AddInt64(&c.ClientQPS, -1)
}

func (c *Counter) IncrErrLogTotal()  {
	atomic.AddInt64(&c.ErrLogTotal, 1)
}

func (c *Counter) IncrSlowLogTotal()  {
	atomic.AddInt64(&c.SlowLogTotal, 1)
}

func (c *Counter) FlushCounter()  {
	atomic.StoreInt64(&c.OldClientQPS, c.ClientQPS)
	atomic.StoreInt64(&c.OldErrLogTotal, c.ErrLogTotal)
	atomic.StoreInt64(&c.OldSlowLogTotal, c.SlowLogTotal)

	atomic.StoreInt64(&c.ClientQPS, 0)
}