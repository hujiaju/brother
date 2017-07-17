package proxyBack

import (
	"brother/config"
	"sync"
	"time"
	"brother/core/golog"
	"sync/atomic"
	"brother/core/errors"
	"strings"
	"strconv"
)

const (
	Master		=	"master"
	Slave		=	"slave"
	SlaveSplit	=	","
	WeightSplit	=	"@"
)

type Node struct {
	Cfg				config.NodeConfig

	sync.RWMutex
	Master				*DB

	Slave				[]*DB
	LastSlaveIndex			int
	RoundRobinQ			[]int
	SlaveWeights			[]int

	DownAfterNoAlive		time.Duration
}

func (n *Node) checkMaster() {
	db := n.Master
	if db == nil {
		golog.Error("Node", "checkMaster", "Master is no alive", 0)
		return 
	}

	if err := db.Ping(); err != nil {
		golog.Error("Node", "checkMaster", "Ping", 0, "db.Addr", db.Addr(), "error", err.Error())
	} else {
		if atomic.LoadInt32(&(db.state)) == Down {
			golog.Info("Node", "checkMaster", "Master up", 0, "db.Addr", db.Addr())
			n.UpMaster(db.addr)
		}
		db.SetLastPing()
		if atomic.LoadInt32(&(db.state)) != ManualDown {
			atomic.StoreInt32(&(db.state), Up)
		}
		return 
	}
	if int64(n.DownAfterNoAlive) > 0 && time.Now().Unix()-db.GetLastPing() > int64(n.DownAfterNoAlive/time.Second) {
		golog.Info("Node", "checkMaster", "Master down", 0,
			"db.Addr", db.Addr(),
			"Master_down_time", int64(n.DownAfterNoAlive/time.Second))
		n.DownMaster(db.addr, Down)
	}
}

func (n *Node) CheckSlave() {
	n.RLock()
	if n.Slave == nil {
		n.RUnlock()
		return
	}
	slaves := make([]*DB, len(n.Slave))
	copy(slaves, n.Slave)
	n.RUnlock()

	for i := 0; i < len(slaves); i ++ {
		if err := slaves[i].Ping(); err != nil {
			golog.Error("Node", "checkSlave", "Ping", 0, "db.Addr", slaves[i].Addr(), "error", err.Error())
		} else {
			if atomic.LoadInt32(&(slaves[i].state)) == Down {
				golog.Info("Node", "checkSlave", "Slave up", 0, "db.Addr", slaves[i].Addr())
				n.UpSlave(slaves[i].addr)
			}
			slaves[i].SetLastPing()
			if atomic.LoadInt32(&(slaves[i].state)) != ManualDown {
				atomic.StoreInt32(&(slaves[i].state), Up)
			}
			continue
		}

		if int64(n.DownAfterNoAlive) > 0 && time.Now().Unix()-slaves[i].GetLastPing() > int64(n.DownAfterNoAlive/time.Second) {
			golog.Info("Node", "checkSlave", "Slave down", 0,
				"db.Addr", slaves[i].Addr(),
				"slave_down_time", int64(n.DownAfterNoAlive/time.Second))
			//If can't ping slave after DownAfterNoAlive, set slave Down
			n.DownSlave(slaves[i].addr, Down)
		}
	}
}

func (n *Node) CheckNode()  {
	//TODO check connection alive
	for {
		n.checkMaster()
		n.CheckSlave()
		time.Sleep(16 * time.Second)
	}
}

func (n *Node) String() string {
	return n.Cfg.Name
}

func (n *Node) GetMasterConn() (*BackendConn, error) {
	db := n.Master
	if db == nil {
		return nil, errors.ErrNoMasterConn
	}

	if atomic.LoadInt32(&(db.state)) == Down {
		return nil, errors.ErrMasterDown
	}

	return db.GetConn()
}

func (n *Node) GetSlaveConn() (*BackendConn, error) {
	n.Lock()
	db, err := n.GetNextSlave()
	n.Unlock()
	if err != nil {
		return nil, err
	}

	if db == nil {
		return nil, errors.ErrNoSlaveDB
	}
	if atomic.LoadInt32(&(db.state)) == Down {
		return nil, errors.ErrSlaveDown
	}

	return db.GetConn()
}

/**
 * ##################################### Up/Down Master/Slave Operations ################################
 */

func (n *Node) OpenDB(addr string) (*DB, error) {
	db, err := Open(addr, n.Cfg.User, n.Cfg.Password, "", n.Cfg.MaxConnNum)
	return db, err
}

func (n *Node) UpDB(addr string) (*DB, error) {
	db, err := n.OpenDB(addr)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		atomic.StoreInt32(&(db.state), Down)
		return nil, err
	}
	atomic.StoreInt32(&(db.state), Up)
	return db, nil
}

func (n *Node) UpMaster(addr string) error {
	db, err := n.UpDB(addr)
	if err != nil {
		golog.Error("Node", "UpMaster", err.Error(), 0)
	}
	n.Master = db
	return err
}

func (n *Node) UpSlave(addr string) error {
	db, err := n.UpDB(addr)
	if err != nil {
		golog.Error("Node", "UpSlave", err.Error(), 0)
	}
	n.Lock()
	for k, slave := range n.Slave {
		if slave.addr == addr {
			n.Slave[k] = db
			n.Unlock()
			return nil
		}
	}
	n.Slave = append(n.Slave, db)
	n.Unlock()

	return err
}

func (n *Node) DownMaster(addr string, state int32) error {
	db := n.Master
	if db == nil || db.addr != addr {
		return errors.ErrNoMasterDB
	}
	db.Close()
	atomic.StoreInt32(&(db.state), state)
	return nil
}

func (n *Node) DownSlave(addr string, state int32) error {
	n.RLock()
	if n.Slave == nil {
		n.RUnlock()
		return errors.ErrNoSlaveDB
	}
	slaves := make([]*DB, len(n.Slave))
	copy(slaves, n.Slave)
	n.RUnlock()

	//slave is *DB
	for _, slave := range slaves {
		if slave.addr == addr {
			slave.Close()
			atomic.StoreInt32(&(slave.state), state)
			break
		}
	}
	return nil
}

func (n *Node) ParseMaster(masterStr string) error {
	var err error
	if len(masterStr) == 0 {
		return errors.ErrNoMasterDB
	}

	n.Master, err = n.OpenDB(masterStr)
	return err
}

//slavesStr(127.0.0.1:3306@2,192.168.10.12:3306)
func (n *Node) ParseSlave(slaveStr string) error {
	var db *DB
	var weight int
	var err error
	if len(slaveStr) == 0 {
		return nil
	}
	slaveStr = strings.Trim(slaveStr, SlaveSplit)
	slaveArray := strings.Split(slaveStr, SlaveSplit)
	count := len(slaveArray)
	n.Slave = make([]*DB, 0, count)
	n.SlaveWeights = make([]int, 0, count)

	//parse addr and port
	for i := 0; i < count; i ++ {
		addrAndWeight := strings.Split(slaveArray[i], WeightSplit)
		if len(addrAndWeight) == 2 {
			weight, err = strconv.Atoi(addrAndWeight[1])
			if err != nil {
				return err
			}
		} else {
			weight = 1
		}
		n.SlaveWeights = append(n.SlaveWeights, weight)
		if db, err = n.OpenDB(addrAndWeight[0]); err != nil {
			return err
		}
		n.Slave = append(n.Slave, db)
	}
	n.InitBalancer()
	return nil
}

func (n *Node) AddSlave(addr string) error {
	var db *DB
	var weight int
	var err error
	if len(addr) == 0 {
		return errors.ErrAddressNull
	}
	n.Lock()
	defer n.Unlock()
	for _, v := range n.Slave {
		if strings.Split(v.addr, WeightSplit)[0] == strings.Split(addr, WeightSplit)[0] {
			return errors.ErrSlaveExist
		}
	}
	addrAndWeight := strings.Split(addr, WeightSplit)
	if len(addrAndWeight) == 2 {
		weight, err = strconv.Atoi(addrAndWeight[1])
		if err != nil {
			 return err
		}
	} else {
		weight = 1
	}

	n.SlaveWeights = append(n.SlaveWeights, weight)
	if db, err = n.OpenDB(addrAndWeight[0]); err != nil {
		return err
	} else {
		n.Slave = append(n.Slave, db)
		n.InitBalancer()
		return nil
	}
}