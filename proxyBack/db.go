package proxyBack

import (
	"sync"
)

type DB struct {
	sync.RWMutex

	addr				string
	user				string
	passwd				string
	db				string
	state				int32

	maxConnNum			int
	InitConnNum			int
	idleConns			chan *Conn
	cacheConns			chan *Conn
	checkConn			*Conn
	lastPing			int64
}