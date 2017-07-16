package proxyBack

import (
	"sync"
	"sync/atomic"
	"time"
	"brother/mysql"
	"brother/core/errors"
)

const (
	Up				= iota
	Down
	ManualDown
	Unknown

	InitConnCount			= 16
	DefaultMaxConnNum		= 1024
	PingPeroid		int64	= 4
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

func Open(addr, user, passwd, dbName string, maxConnNum int) (*DB, error) {
	var err error
	db := new(DB)
	db.addr = addr
	db.user = user
	db.passwd = passwd
	db.db = dbName

	if maxConnNum > 0 {
		db.maxConnNum = maxConnNum
		if db.maxConnNum < 16 {
			db.InitConnNum = db.maxConnNum
		} else {
			db.InitConnNum = db.maxConnNum / 4
		}
	} else {
		db.maxConnNum = DefaultMaxConnNum
		db.InitConnNum = InitConnCount
	}
	//check connection 建立与mysql 数据库的真正连接
	db.checkConn, err = db.newConn()
	if err != nil {
		db.Close()
		return nil, err
	}

	db.idleConns = make(chan *Conn, db.maxConnNum)
	db.cacheConns = make(chan *Conn, db.maxConnNum)
	atomic.StoreInt32(&(db.state), Unknown)

	for i := 0; i < db.maxConnNum ; i++ {
		if i < db.InitConnNum {
			conn, err := db.newConn()
			if err != nil {
				db.Close()
				return nil, err
			}
			conn.pushTimestamp = time.Now().Unix()
			db.cacheConns <- conn
		} else {
			conn := new(Conn)
			db.idleConns <- conn
		}
	}
	db.SetLastPing()

	return db, nil
}

func (db *DB) Close() error {
	db.RLock()
	idleChannel := db.idleConns
	cacheChannel := db.cacheConns
	db.cacheConns = nil
	db.idleConns = nil
	db.RUnlock()
	if cacheChannel == nil || idleChannel == nil {
		return nil
	}

	close(cacheChannel)
	for conn := range cacheChannel{
		db.closeConn(conn)
	}
	close(idleChannel)

	return nil
}

/**
 * ########################################## DB Getter #############################################
 */
func (db *DB) Addr() string {
	return db.addr
}

func (db *DB) getConns() (chan *Conn, chan *Conn) {
	db.RLock()
	cacheConns := db.cacheConns
	idleConns := db.idleConns
	db.RUnlock()
	return cacheConns, idleConns
}

func (db *DB) getCacheConns() chan *Conn {
	db.RLock()
	conns := db.cacheConns
	db.RUnlock()
	return conns
}

func (db *DB) getIdleConns() chan *Conn {
	db.RLock()
	conns := db.idleConns
	db.RUnlock()
	return conns
}

/**
 * ########################################## DB Conn Managment #############################################
 */

func (db *DB) Ping() error {

	return nil
}

func (db *DB) newConn() (*Conn, error) {
	co := new(Conn)

	if err := co.Connect(db.addr, db.user, db.passwd, db.db); err != nil {
		return nil, err
	}
	return co, nil
}

func (db *DB) closeConn(co *Conn) error {
	if co != nil {
		co.Close()
		conns := db.getIdleConns()
		if conns != nil {
			select {
			case conns <- co:
				return nil
			default:
				return nil
			}
		}
	}
	return nil
}

func (db *DB) tryReuse(co *Conn) error {
	var err error
	//reuse Connection
	if co.IsInTransaction() {
		//we can not reuse a connection in transaction status
		err = co.Rollback()
		if err != nil {
			return err
		}
	}

	if !co.IsAutoCommit() {
		//we can not resue a connection not in autocommit
		_, err = co.exec("set autocommit = 1")
		if err != nil {
			return err
		}
	}

	//connection may be set names early
	//we must use default utf-8
	if co.GetCharset() != mysql.DEFAULT_CHARSET {
		err = co.SetCharset(mysql.DEFAULT_CHARSET, mysql.DEFAULT_COLLATION_ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) PopConn() (*Conn, error) {
	var co *Conn
	var err error

	cacheConns, idleConns := db.getConns()
	if cacheConns == nil || idleConns == nil {
		return nil, errors.ErrDatabaseClose
	}
	co = db.GetConnFromCache(cacheConns)
	if co == nil {
		co, err = db.GetConnFromIdle(cacheConns, idleConns)
		if err != nil {
			return nil, err
		}
	}

	err = db.tryReuse(co)
	if err != nil {
		db.closeConn(co)
		return nil, err
	}
	return co, nil
}

func (db *DB) GetConnFromCache(cacheConns chan *Conn) (*Conn) {
	var co *Conn
	var err error
	for len(cacheConns) > 0 {
		co = <- cacheConns
		if co != nil && PingPeroid < time.Now().Unix()-co.pushTimestamp {
			err = co.Ping()
			if err != nil {
				db.closeConn(co)
				co = nil
			}
		}
		if co != nil {
			break
		}
	}
	return co
}

func (db *DB) GetConnFromIdle(cacheConns, idleConns chan *Conn) (*Conn, error) {
	var co *Conn
	var err error
	select {
	case co = <- idleConns:
		err = co.Connect(db.addr, db.user, db.passwd, db.db)
		if err != nil {
			db.closeConn(co)
			return nil, err
		}
		return co, nil
	case co = <- cacheConns:
		if co == nil {
			return nil, errors.ErrConnIsNil
		}
		if co != nil && PingPeroid < time.Now().Unix()-co.pushTimestamp {
			err = co.Ping()
			if err != nil {
				db.closeConn(co)
				return nil, errors.ErrBadConn
			}
		}
	}
	return co, nil
}

func (db *DB) PushConn(co *Conn, err error) {
	if co == nil {
		return
	}

	conns := db.getCacheConns()
	if conns == nil {
		co.Close()
		return
	}
	if err != nil {
		db.closeConn(co)
		return
	}

	co.pushTimestamp = time.Now().Unix()
	select {
	case conns <- co:
		return
	default:
		db.closeConn(co)
		return
	}
}


/**
 * ################################# BackConn Struct ################################################
 */

type BackendConn struct {
	*Conn
	db *DB
}

func (p *BackendConn) Close()  {
	if p != nil && p.Conn != nil {
		if p.Conn.pkgErr != nil {
			p.db.closeConn(p.Conn)
		} else {

		}
		p.Conn = nil
	}
}

func (db *DB) SetLastPing() {
	db.lastPing = time.Now().Unix()
}

func (db *DB) GetLastPing() int64 {
	return db.lastPing
}

func (db *DB) GetConn() (*BackendConn, error) {
	c, err := db.PopConn()
	if err != nil {
		return nil, err
	}
	return &BackendConn{c, db}, nil
}