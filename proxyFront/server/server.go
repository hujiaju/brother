package proxy

import (
	"brother/config"
	"net"
)

type Schema struct {
	//nodes				map[string]*back
	//rule				*router
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

	allowips			[2][]net.IP

	schema				*Schema

	listener			net.Listener
	running				bool
}

func (s *Server) Run()  {

}
