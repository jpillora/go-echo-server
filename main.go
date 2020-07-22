package main

import (
	"log"
	"net/http"
	"strconv"

	echo "github.com/jpillora/go-echo-server/handler"
	"github.com/jpillora/go-echo-server/udp"
	"github.com/jpillora/opts"
)

var version = "0.0.0-src"

func main() {
	c := struct {
		Port        int  `help:"Port" env:"PORT"`
		UDP         bool `help:"UDP mode"`
		echo.Config `type:"embedded"`
	}{
		Port: 3000,
	}
	opts.New(&c).
		Name("go-echo-server").
		Version(version).
		Repo("github.com/jpillora/go-echo-server").
		Parse()
	//udp mode?
	if c.UDP {
		log.Fatal(udp.Start(c.Port))
	}
	//http mode
	h := echo.New(c.Config)
	log.Printf("Listening for http requests on %d...", c.Port)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(c.Port), h))
}
