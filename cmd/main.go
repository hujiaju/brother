package main

import (
	f"fmt"
	"runtime"
	"flag"

	."mysql"
)

/**
 * banner
 */
var banner = `
 ____  _____  ____  ____  ____  _____  _     _____ ____
/  __\/__ __\/  _ \/  __\/  _ \/__ __\/ \ /|/  __//  __\
|  \/|  / \  | | //|  \/|| / \|  / \  | |_|||  \  |  \/|
|  __/  | |  | |_\\|    /| \_/|  | |  | | |||  /_ |    /
\_/     \_/  \____/\_/\_\\____/  \_/  \_/ \|\____\\_/\_\

`

var configFile *string = flag.String("config", "/Users/nanhujiaju/Desktop/GitHubs/kingshard/etc/ks.yaml", "kingshard config file")


func main() {

	f.Println(banner)
	runtime.GOMAXPROCS(runtime.NumCPU())
	flag.Parse()

	var m = CLIENT_COMPRESS
	f.Println(m)

}
