package main

import (
	f"fmt"
	"runtime"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"brother/config"
	"brother/proxyFront/server"
	"brother/core/golog"
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

var configFile *string = flag.String("config", "/Users/nanhujiaju/Desktop/GitHubs/kingshard/etc/ks.yaml", "brother config file")
var logLevel *string = flag.String("log-level", "", "log level [debug|info|warn|error], default error")

func main() {

	f.Println(banner)
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()

	if len(*configFile) == 0 {
		f.Println("must use a config file")
		return
	}
	cfg, err := config.ParseConfigFile(*configFile)
	if err != nil {
		f.Printf("parse config file error:%v\n", err.Error())
		return
	}

	var svr *server.Server
	svr, err = server.NewServer(cfg)
	if err != nil {
		golog.Error("main", "main", err.Error(), 0)
		golog.GlobalSysLogger.Close()
		golog.GlobalSqlLogger.Close()
		return
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGPIPE,
	)
	go func() {
		for {
			sig := <- sc
			if sig == syscall.SIGINT || sig == syscall.SIGTERM || sig == syscall.SIGQUIT {
				golog.Info("main", "main", "Got signal", 0, "signal", sig)
				golog.GlobalSysLogger.Close()
				golog.GlobalSqlLogger.Close()
				svr.Close()
			} else if sig == syscall.SIGPIPE{
				golog.Info("main", "main", "Ignore broken pipe signal", 0)
			}
		}
	}()
	svr.Run()
}
