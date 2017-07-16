package proxyBack

import (
	"math/rand"
	"time"
	"brother/core/errors"
)

func GCD(ary []int) int {
	var i int
	min := ary[0]
	length := len(ary)
	for i = 0; i < length; i++ {
		if ary[i] < min {
			min = ary[i]
		}
	}

	for {
		isCommon := true
		for i = 0; i < length; i++ {
			if ary[i]%min != 0 {
				isCommon = false
				break
			}
		}
		if isCommon {
			break
		}
		min--
		if min < 1 {
			break
		}
	}
	return min
}

func (n *Node) InitBalancer() {
	var sum int
	n.LastSlaveIndex = 0
	gcd := GCD(n.SlaveWeights)

	for _, weight := range n.SlaveWeights {
		sum += weight / gcd
	}

	n.RoundRobinQ = make([]int, 0, sum)
	for index, weight := range n.SlaveWeights {
		for j := 0; j < weight / gcd; j++ {
			n.RoundRobinQ = append(n.RoundRobinQ, index)
		}
	}
	//random order
	if len(n.SlaveWeights) > 1 {
		r := rand.New(rand.NewSource(time.Now().UnixNano()))
		for i := 0; i < sum; i ++ {
			x := r.Intn(sum)
			tmp := n.RoundRobinQ[x]
			other := sum % (x + 1)
			n.RoundRobinQ[x] = n.RoundRobinQ[other]
			n.RoundRobinQ[other] = tmp
		}
	}
}

func (n *Node) GetNextSlave() (*DB, error) {
	var index int
	queueLen := len(n.RoundRobinQ)
	if queueLen == 0 {
		return nil, errors.ErrNoDatabase
	}
	if queueLen == 1 {
		index = n.RoundRobinQ[0]
		return n.Slave[index], nil
	}

	n.LastSlaveIndex = n.LastSlaveIndex % queueLen
	index = n.RoundRobinQ[n.LastSlaveIndex]
	if len(n.Slave) <= index {
		return nil, errors.ErrNoDatabase
	}
	db := n.Slave[index]
	n.LastSlaveIndex++
	n.LastSlaveIndex = n.LastSlaveIndex % queueLen
	return db, nil
}
