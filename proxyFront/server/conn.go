package server

import (
	"sync"
	"brother/mysql"
	"net"
	"brother/proxyBack"
	f"fmt"
	"brother/core/golog"
	"encoding/binary"
	"bytes"
	"runtime"
)

//front client <-> mysql proxy
type ClientConn struct {
	sync.Mutex

	pkg				*mysql.PacketIO

	c				net.Conn

	proxy				*Server

	capability			uint32

	connectionId			uint32

	status				uint16
	collation			mysql.CollationId
	charset				string

	user				string
	db				string

	salt				[]byte

	schema				*Schema

	txConns				map[*proxyBack.Node]*proxyBack.BackendConn

	closed				bool

	lastInsertId			int64
	affectedRows			int64

	stmtId				uint32

	stmts				map[uint32]*Stmt //prepare操作（可防止SQL注入）client端到proxy的stmt
}

var baseConnId uint32 = 10000

var DEFAULT_CAPABILITY uint32 = mysql.CLIENT_LONG_PASSWORD | mysql.CLIENT_LONG_FLAG |
mysql.CLIENT_CONNECT_WITH_DB | mysql.CLIENT_PROTOCOL_41 |
mysql.CLIENT_TRANSACTIONS | mysql.CLIENT_SECURE_CONNECTION

func (c *ClientConn) IsAllowConnect() bool {
	clientHost, _, err := net.SplitHostPort(c.c.RemoteAddr().String())
	if err != nil {
		f.Println(err)
	}
	clientIP := net.ParseIP(clientHost)

	ipVec := c.proxy.allowips[c.proxy.allowipsIndex]
	if ipVecLen := len(ipVec); ipVecLen == 0 {
		return true
	}
	for _, ip := range ipVec {
		if ip.Equal(clientIP) {
			return true
		}
	}
	golog.Error("server", "IsAllowConnect", "error", mysql.ER_ACCESS_DENIED_ERROR,
		"ip address", c.c.RemoteAddr().String(), " access denied by brother.")
	return false
}

func (c *ClientConn) Handshake() error {
	if err := c.writeInitialHandshake(); err != nil {
		golog.Error("server", "Handshake", err.Error(),
			c.connectionId, "msg", "send initial handshake error")
		return err
	}

	if err := c.readHandshakeResponse(); err != nil {
		golog.Error("server", "readHandshakeResponse",
			err.Error(), c.connectionId,
			"msg", "read Handshake Response error")
		return err
	}

	if err := c.writeOK(nil); err != nil{
		golog.Error("server", "readHandshakeResponse",
			"write ok fail",
			c.connectionId, "error", err.Error())
		return err
	}

	c.pkg.Sequence = 0
	return nil
}

/**
 * ######################################## Conn write/read event ###########################
 */

func (c *ClientConn) writeInitialHandshake() error {
	data := make([]byte, 4, 128)

	//min version 10
	data = append(data, 10)

	//server version[00]
	data = append(data, mysql.ServerVersion...)
	data = append(data, 0)

	//connection id
	data = append(data, byte(c.connectionId), byte(c.connectionId>>8), byte(c.connectionId>>16), byte(c.connectionId>>24))

	//auth-plugin-data-part-1
	data = append(data, c.salt[0:8]...)

	//filter [00]
	data = append(data, 0)

	//capability flag lower 2 bytes, using default capability here
	data = append(data, byte(DEFAULT_CAPABILITY), byte(DEFAULT_CAPABILITY>>8))

	//charset, utf-8 default
	data = append(data, uint8(mysql.DEFAULT_COLLATION_ID))

	//status
	data = append(data, byte(c.status), byte(c.status>>8))

	//below 13 byte may not be used
	//capability flag upper 2 bytes, using default capability here
	data = append(data, byte(DEFAULT_CAPABILITY>>16), byte(DEFAULT_CAPABILITY>>24))

	//filter [0x15], for wireshark dump, value is 0x15
	data = append(data, 0x15)

	//reserved 10 [00]
	data = append(data, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)

	//auth-plugin-data-part2
	data = append(data, c.salt[8:]...)

	//filter [00]
	data = append(data, 0)

	return c.writePacket(data)
}

func (c *ClientConn) readHandshakeResponse() error {
	data, err := c.readPacket()
	if  err != nil {
		return err
	}
	//读取数据的位置 标志
	pos := 0

	//capability
	c.capability = binary.LittleEndian.Uint32(data[:4])
	pos += 4

	//skip max packet size
	pos += 4

	//charset, skip, if you want to use another charset, use set names
	//c.collation = Collation(data[pos])
	pos++

	//skip reserved 23[00]
	pos += 23

	//user name
	c.user = string(data[pos:pos+bytes.IndexByte(data[pos:], 0)])
	pos += len(c.user) + 1

	//auth length and auth
	authLen := int(data[pos])
	pos++
	auth := data[pos:pos+authLen]

	checkAuth := mysql.CalcPassword(c.salt, []byte(c.proxy.cfg.Password))
	if c.user != c.proxy.cfg.User || !bytes.Equal(auth, checkAuth) {
		golog.Error("ClientConn", "readHandshakeResponse", "error", 0,
			"auth", auth,
			"checkAuth", checkAuth,
			"client_user", c.user,
			"config_set_user", c.proxy.cfg.User,
			"passworld", c.proxy.cfg.Password)
		return mysql.NewDefaultError(mysql.ER_ACCESS_DENIED_ERROR, c.user, c.c.RemoteAddr().String(), "YES")
	}

	pos += authLen

	var db string
	if c.capability&mysql.CLIENT_CONNECT_WITH_DB >0 {
		if len(data[pos:]) == 0 {
			return nil
		}

		db = string(data[pos:pos+bytes.IndexByte(data[pos:], 0)])
		pos += len(c.db) + 1
	}
	c.db = db

	return nil
}

func (c *ClientConn) writePacket(data []byte) error {
	return c.pkg.WritePacket(data)
}

func (c *ClientConn) readPacket() ([]byte, error) {
	return c.pkg.ReadPacket()
}

func (c *ClientConn) writePacketBatch(total, data []byte, direct bool) ([]byte, error) {
	return c.pkg.WritePacketBatch(total, data, direct)
}

func (c *ClientConn) writeOK(r *mysql.Result) error {
	if r == nil {
		r = &mysql.Result{Status:c.status}
	}
	data := make([]byte, 4, 32)

	data = append(data, mysql.OK_HEADER)

	data = append(data, mysql.PutLengthEncodedInt(r.AffectedRows)...)
	data = append(data, mysql.PutLengthEncodedInt(r.InsertId)...)

	if c.capability&mysql.CLIENT_PROTOCOL_41 > 0 {
		data = append(data, byte(r.Status), byte(r.Status>>8))
		data = append(data, 0)
	}

	return c.writePacket(data)
}

func (c *ClientConn) writeError(e error) error {
	var m *mysql.SqlError
	var ok bool
	if m, ok = e.(*mysql.SqlError); !ok {
		m = mysql.NewError(mysql.ER_UNKNOWN_ERROR, e.Error())
	}

	data := make([]byte, 4, 16+len(m.Message))

	data = append(data, mysql.ERR_HEADER)
	data = append(data, byte(m.Code), byte(m.Code>>8))

	if c.capability&mysql.CLIENT_PROTOCOL_41 >0 {
		data = append(data, '#')
		data = append(data, m.State...)
	}

	data = append(data, m.Message...)
	return c.writePacket(data)
}

func (c *ClientConn) writeEOF(status uint16) error {
	data := make([]byte, 4, 9)
	
	data = append(data, mysql.EOF_HEADER)
	if c.capability&mysql.CLIENT_PROTOCOL_41 >0 {
		data = append(data, 0, 0)
		data = append(data, byte(status), byte(status>>8))
	}

	return c.writePacket(data)
}

func (c *ClientConn) writeEOFBatch(total []byte, status uint16, direct bool) ([]byte, error) {
	data := make([]byte, 4, 9)
	
	data = append(data, mysql.EOF_HEADER)
	if c.capability&mysql.CLIENT_PROTOCOL_41 > 0 {
		data = append(data, 0, 0)
		data = append(data, byte(status), byte(status>>8))
	}

	return c.writePacketBatch(total, data, direct)
}

func (c *ClientConn) Close() error {
	if c.closed {
		return nil
	}

	c.c.Close()
	c.closed = true

	return nil
}

func (c *ClientConn) Run() {
	defer func() {
		r := recover()
		if err, ok := r.(error); ok {
			const size = 4096
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			golog.Error("ClientConn", "Run",
				err.Error(), 0,
				"stack", string(buf))
		}
		c.Close()
	}()

	for {
		data, err := c.readPacket()
		if err != nil {
			return
		}

		if err := c.dispatch(data); err != nil{
			c.proxy.counter.IncrErrLogTotal()
			golog.Error("server", "Run",
				err.Error(), c.connectionId,
			)
			c.writeError(err)
			if err == mysql.ErrBadConn {
				c.Close()
			}
		}

		if c.closed {
			return
		}

		c.pkg.Sequence = 0
	}
}

func (c *ClientConn) dispatch(data []byte) error {
	c.proxy.counter.IncrClientQPS()
	cmd := data[0]
	data = data[1:]

	golog.Info("server", "dispatch", "received cmd:", 0, cmd)
	switch cmd {
	case mysql.COM_QUIT:

	default:
		msg := f.Sprintf("command %d not supported now", cmd)
		golog.Error("ClientConn", "dispatch", msg, 0)
		return mysql.NewError(mysql.ER_UNKNOWN_ERROR, msg)
	}

	return nil
}