package proxyBack

type Stmt struct {
	conn				*Conn
	id				uint32
	query				string

	params				int
	columns				int
}