package server

import (
	"brother/config"
	"net"
	"brother/proxyBack"
	f"fmt"
	"sync/atomic"
	"strings"
	"brother/mysql"
	"brother/core/errors"
	"brother/core/golog"
	"time"
	"runtime"
	"os"
	"bufio"
	"io"
)

/**
 * 分库分表规则
 */
type Schema struct {
	nodes				map[string]*proxyBack.Node
	//rule				*router
}

type BlacklistSqls struct {
	sqls 				map[string]string
	sqlsLen				int
}

const (
	Offline		= iota
	Online
	Unknown
)

type Server struct {
	cfg				*config.Config	//配置
	addr				string
	user				string
	passwd				string

	statusIndex			int32
	status				[2]int32

	blacklistSqlsIndex		int32
	blacklistSqls			[2]*BlacklistSqls

	allowipsIndex			int32
	allowips			[2][]net.IP

	counter				*Counter
	nodes				map[string]*proxyBack.Node
	schema				*Schema

	listener			net.Listener
	running				bool
}

func (s *Server) Status() string  {
	var status string
	switch s.status[s.statusIndex] {
	case Online:
		status = "online"
	case Offline:
		status = "offline"
	case Unknown:
		status = "unknown"
	default:
		status = "unknown"
	}
	return status
}

func (s *Server) parseAllowIps() error {
	atomic.StoreInt32(&s.allowipsIndex, 0)

	cfg := s.cfg
	if len(cfg.AllowIps) == 0 {
		return nil
	}
	ipVec := strings.Split(cfg.AllowIps, ",")
	s.allowips[s.allowipsIndex] = make([]net.IP, 0, 10)
	s.allowips[1] = make([]net.IP, 0, 10)
	for _, ip := range ipVec{
		s.allowips[s.allowipsIndex] = append(s.allowips[s.allowipsIndex], net.ParseIP(strings.TrimSpace(ip)))
	}
	return nil
}

/**
 * #############################################server parser######################################################
 **/

func (s *Server) parseBlackListSqls() error {
	//TODO to be continue
	bs := new(BlacklistSqls)
	bs.sqls = make(map[string]string)
	if len(s.cfg.BlsFile) != 0 {
		file, err := os.Open(s.cfg.BlsFile)
		if err != nil {
			return err
		}

		defer file.Close()
		rd := bufio.NewReader(file)
		for {
			line, err := rd.ReadString('\n')
			//end of file
			if err == io.EOF {
				if len(line) != 0 {
					fingerPrint := mysql.GetFingerprint(line)
					md5 := mysql.GetMd5(fingerPrint)
					bs.sqls[md5] = fingerPrint
				}
				break
			}
			if err != nil {
				return err
			}
			line = strings.TrimSpace(line)
			if len(line) != 0 {
				fingerPrint := mysql.GetFingerprint(line)
				md5 := mysql.GetMd5(fingerPrint)
				bs.sqls[md5] = fingerPrint
			}
		}
	}
	bs.sqlsLen = len(bs.sqls)
	atomic.StoreInt32(&s.blacklistSqlsIndex, 0)
	s.blacklistSqls[s.blacklistSqlsIndex] = bs
	s.blacklistSqls[1] = bs
	
	return nil
}

func (s *Server) parseNode(cfg config.NodeConfig) (*proxyBack.Node, error) {
	var err error
	n := new(proxyBack.Node)
	n.Cfg = cfg

	n.DownAfterNoAlive = time.Duration(cfg.DownAfterNoAlive) * time.Second
	err = n.ParseMaster(cfg.Master)
	if err != nil {
		return nil, err
	}
	err = n.ParseSlave(cfg.Slave)
	if err != nil {
		return nil, err
	}

	go n.CheckNode()

	return n, nil
}

func (s *Server) parseNodes() error {
	cfg := s.cfg
	s.nodes = make(map[string]*proxyBack.Node, len(cfg.Nodes))

	for _, v := range cfg.Nodes {
		if _, ok := s.nodes[v.Name]; ok {
			return f.Errorf("duplicate node [%s].", v.Name)
		}
		n, err := s.parseNode(v)
		if err != nil {
			return nil
		}
		s.nodes[v.Name] = n
	}
	
	return nil
}

func (s *Server) parseSchema() error {
	schemaCfg := s.cfg.Schema
	if len(schemaCfg.Nodes) == 0 {
		return f.Errorf("schema must have a node.")
	}

	nodes := make(map[string]*proxyBack.Node)
	for _, n := range  schemaCfg.Nodes {
		if s.GetNode(n) == nil {
			return f.Errorf("schema node [%s] config is not exists.", n)
		}
		if _, ok := nodes[n]; ok {
			return f.Errorf("schema node [%s] duplicated.", n)
		}

		nodes[n] = s.GetNode(n)
	}

	//TODO 暂时不实现路由

	return nil
}

/**
 * #############################################server gettter######################################################
 **/

func (s *Server) GetSchema() *Schema {
	return s.schema
}

func (s *Server) GetNode(name string) *proxyBack.Node {
	return s.nodes[name]
}

func (s *Server) GetAllNodes() map[string]*proxyBack.Node {
	return s.nodes
}

func (s *Server) GetAllowIps() []string {
	var ips []string
	for _, v := range  s.allowips[s.allowipsIndex] {
		if v != nil {
			ips = append(ips, v.String())
		}
	}
	return ips
}

/**
 * #############################################server event######################################################
 **/

func NewServer(cfg *config.Config) (*Server, error) {
	s := new(Server)

	s.cfg = cfg
	s.counter = new(Counter)
	s.addr = cfg.Addr
	s.user = cfg.User
	s.passwd = cfg.Password
	atomic.StoreInt32(&s.statusIndex, 0)
	s.status[s.statusIndex] = Online
	//atomic.StoreInt32(&s.logSqlIndex, 0)
	//s.logSql[s.logSqlIndex] = cfg.LogSql
	//atomic.StoreInt32(&s.slowLogTimeIndex, 0)
	//s.slowLogTime[s.slowLogTimeIndex] = cfg.SlowLogTime

	if len(cfg.Charset) == 0 {
		cfg.Charset = mysql.DEFAULT_CHARSET //utf8
	}
	cid, ok := mysql.CharsetIds[cfg.Charset]
	if !ok {
		return nil, errors.ErrInvalidCharset
	}
	//change the default charset
	mysql.DEFAULT_CHARSET = cfg.Charset
	mysql.DEFAULT_COLLATION_ID = cid
	mysql.DEFAULT_COLLATION_NAME = mysql.Collations[cid]

	if err := s.parseBlackListSqls(); err != nil {
		return nil, err
	}

	if err := s.parseAllowIps(); err != nil {
		return nil, err
	}

	if err := s.parseNodes(); err != nil {
		return nil, err
	}
	//忽略分表规则
	//if err := s.parseSchema(); err != nil {
	//	return nil, err
	//}

	var err error
	netProto := "tcp"
	s.listener, err = net.Listen(netProto, s.addr)
	if err != nil {
		return nil, err
	}

	golog.Info("server", "NewServer", "Server running", 0,
		"netProto",
		netProto,
		"address",
		s.addr)
	return s, nil
}

func (s *Server) flushCounter()  {
	for {
		s.counter.FlushCounter()
		time.Sleep(1 * time.Second)
	}
}

func (s *Server) newClientConn(co net.Conn) *ClientConn  {
	c := new(ClientConn)
	tcpConn := co.(*net.TCPConn)
	//SetNoDelay controls whether the operating system should delay packet transmission
	// in hopes of sending fewer packets (Nagle's algorithm).
	// The default is true (no delay),
	// meaning that data is sent as soon as possible after a Write.
	//I set this option false.
	tcpConn.SetNoDelay(false)
	c.c = tcpConn

	c.schema = s.GetSchema()

	c.pkg = mysql.NewPacketIO(tcpConn)
	c.proxy = s

	c.pkg.Sequence = 0

	c.connectionId = atomic.AddUint32(&baseConnId, 1)

	c.status = mysql.SERVER_STATUS_AUTOCOMMIT

	c.salt = mysql.RandomBuf(20)

	c.txConns = make(map[*proxyBack.Node]*proxyBack.BackendConn)

	c.closed = false

	c.charset = mysql.DEFAULT_CHARSET
	c.collation = mysql.DEFAULT_COLLATION_ID

	c.stmtId = 0
	c.stmts = make(map[uint32]*Stmt)

	return c
}

func (s *Server) Close() {
	s.running = false
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *Server) Run() error {
	f.Println("server running")

	s.running = true

	//flush counter
	go s.flushCounter()

	for s.running {
		conn, err := s.listener.Accept()
		if err != nil {
			golog.Error("Server", "Run", err.Error(), 0)
			continue
		}

		go s.onConn(conn)
	}
	return nil
}

func (s *Server) onConn(c net.Conn) {
	s.counter.IncrClientConns()
	conn := s.newClientConn(c)//新建一个client<->proxy的连接

	defer func() {
		err := recover()
		if err != nil {
			const size  = 4096
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)] //获得当前goroutine的stacktrace 堆栈信息
			golog.Error("server", "onConn", "error", 0, "remoteAddr", c.RemoteAddr().String(), "stack", string(buf))
		}

		conn.Close()
		s.counter.DecrClientConns()
	}()

	if allowConnect := conn.IsAllowConnect(); allowConnect == false {
		err := mysql.NewError(mysql.ER_ACCESS_DENIED_ERROR, "ip address access denied by brother!")
		conn.writeError(err)
		conn.Close()
		return
	}
	if err := conn.Handshake(); err != nil {
		golog.Error("server", "onConn", err.Error(), 0)
		conn.writeError(err)
		conn.Close()
		return
	}

	conn.Run()
}

/**
 * ############################################# web server api events ######################################################
 **/