package main

import (
	f"fmt"
	"runtime"
	"flag"

	."mysql"
	_ "brother/proxyFront/server"
)

/**
 * banner
 */
var banner = `       _   _               _   _
 _ __ | |_| |__  _ __ ___ | |_| |__   ___ _ __
| '_ \| __| '_ \| '__/ _ \| __| '_ \ / _ \ '__|
| |_) | |_| |_) | | | (_) | |_| | | |  __/ |
| .__/ \__|_.__/|_|  \___/ \__|_| |_|\___|_|
|_|
`

var configFile *string = flag.String("config", "/Users/nanhujiaju/Desktop/GitHubs/kingshard/etc/ks.yaml", "kingshard config file")


func main() {

	f.Println(banner)
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()

	var m = CLIENT_COMPRESS
	f.Println(m)

	//var svr *server.Server

	svr.Run()
}
