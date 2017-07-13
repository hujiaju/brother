package main

import (
	f"fmt"
	"database/sql"
	_"github.com/go-sql-driver/mysql"
	"encoding/json"

)

// 结构体成员仅大写开头外界才能访问
type User struct {
	User      string    `json:"user"`
	Password string `json:"password"`
	Host   string `json:"host"`
}

func main() {

	f.Println("hello")

	//prepare const
	protocol := "tcp"
	admin_user := "root"
	admin_pwd := "letmein"
	addr := "127.0.0.1:3306"
	param := f.Sprintf("%s:%s@%s(%s)/mysql?charset=utf8mb4&timeout=30s", admin_user, admin_pwd, protocol, addr)
	f.Println("params is :", param)
	var err error
	db, err := sql.Open("mysql", param)
	if err != nil {
		f.Println("error", err.Error())
		panic(err.Error())
	}
	defer db.Close()

	//ping the mysql db
	err = db.Ping()
	if err != nil {
		f.Println("ping error :", err)
	}

	//db.SetConnMaxLifetime()

	//_, err = db.Exec("show databases")
	//if err != nil {
	//	//panic(err.Error())
	//	f.Println("err",err)
	//}

	db.Query("show databases")


	err = db.Ping()
	if err != nil{
		f.Println(err)
	}

	// 提醒一句, 运行到这里, 并不代表数据库连接是完全OK的, 因为发送第一条SQL才会校验密码 汗~!
	_, e2 := db.Query("select 1")
	if e2 == nil {
		println("DB OK")
		rows, e := db.Query("select user,host from mysql.user")
		if e != nil {
			f.Print("query error!!%v\n", e)
			return
		}
		defer rows.Close()
		if rows == nil {
			print("Rows is nil")
			return
		}
		for rows.Next() { //跟java的ResultSet一样,需要先next读取
			user := new(User)
			// rows貌似只支持Scan方法 继续汗~! 当然,可以通过GetColumns()来得到字段顺序
			row_err := rows.Scan(&user.User, &user.Host)
			if row_err != nil {
				print("Row error!!")
				return
			}
			b, _ := json.Marshal(user)
			f.Println(string(b)) // 这里没有判断错误, 呵呵, 一般都不会有错吧
		}
		println("Done")
	}



}
