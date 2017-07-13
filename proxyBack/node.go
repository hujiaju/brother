package proxyBack

import (
	"brother/config"
	"sync"
	"time"
)

const (
	Master		=	"master"
	Slave		=	"slave"
)

type Node struct {
	cfg				config.NodeConfig

	sync.RWMutex
	Master				*DB

	Slave				[]*DB
	LastSlaveIndex			int
	RoundRobinQ			[]int
	SlaveWeights			[]int

	DownAfterNoAlive		time.Duration
}