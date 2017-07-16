package server

import (
	"fmt"
	"brother/mysql"
	"brother/proxyBack"
)

func (c *ClientConn) handleUseDB(dbName string) error {
	var co *proxyBack.BackendConn
	var err error

	if len(dbName) == 0 {
		return fmt.Errorf("must have database, the length of dbName is zero")
	}
	if c.schema == nil {
		return mysql.NewDefaultError(mysql.ER_NO_DB_ERROR)
	}

	//TODO 找到默认的节点 此处先写死为 node1
	//nodeName := c.schema.rule.DefaultRule.Nodes[0]
	nodeName := "node1"

	n := c.proxy.GetNode(nodeName)
	//get the connection from slave preferentially
	co, err = n.GetSlaveConn()
	if err != nil {
		co, err = n.GetMasterConn()
	}
	defer c.closeConn(co, false)
	if err != nil {
		return err
	}

	if err = co.UseDB(dbName); err != nil {
		//reset the client database to null
		c.db = ""
		return err
	}
	c.db = dbName
	return c.writeOK(nil)
}