package server

import (
	"brother/core/golog"
	"runtime"
	"strings"
	"brother/sqlparser"
	"brother/proxyBack"
	f"fmt"
)

/**
 * ################################### handle SQL 语句 proxy <-> mysql server ###########################################
 */
func (c *ClientConn) handleQuery(sql string) (err error) {
	defer func() {
		if e := recover(); e != nil {
			golog.OutputSql("Error", "err:%v,sql:%s", e, sql)

			if err, ok := e.(error); ok {
				const size = 4096
				buf := make([]byte, size)
				buf = buf[:runtime.Stack(buf, false)]

				golog.Error("ClientConn", "handleQuery",
					err.Error(), 0,
					"stack", string(buf), "sql", sql)
			}
			return
		}
	}()

	sql = strings.TrimRight(sql, ";") //删除sql语句最后的分号
	//TODO 此处不再处理 分表

	var stmt sqlparser.Statement
	stmt, err = sqlparser.Parse(sql) //解析sql语句， 得到的stmt是一个interface 类型
	if err != nil {
		golog.Error("server", "parse", err.Error(), 0, "hasHandled", "", "sql", sql)
		return err
	}

	switch v := stmt.(type) {
	case *sqlparser.Select:
		f.Println("Select cmd!")
		return nil
	case *sqlparser.Insert:
		f.Println("insert cmd!")
		return nil
	case *sqlparser.Update:
		f.Println("update cmd!")
		return nil
	case *sqlparser.Delete:
		f.Println("delete cmd!")
		return nil
	case *sqlparser.Replace:
		f.Println("replace cmd!")
		return nil
	case *sqlparser.UseDB:
		f.Println("use cmd!")
		return nil
	default:
		f.Println("value", v)
		return f.Errorf("statement %T not support now", stmt)
	}

	return nil
}

func (c *ClientConn) closeConn(conn *proxyBack.BackendConn, rollback bool) {
	if c.isInTransaction() {
		return
	}

	if rollback {
		conn.Rollback()
	}

	conn.Close()
}