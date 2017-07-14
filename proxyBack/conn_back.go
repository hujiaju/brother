package proxyBack

import (
	"net"
	"brother/mysql"
)

//proxy <-> mysql server
type Conn struct {
	conn 				net.Conn

	pkg 				*mysql.PacketIO

	addr				string
	user				string
	passwd				string
	db				string

	capability			uint32

	status				uint16

	collation			mysql.CollationId
	charset				string
	salt				[]byte

	pushTimestamp			int64
	pkgErr				error
}
