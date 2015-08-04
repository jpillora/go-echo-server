package main

import (
	"log"
	"net/http"
	"strconv"

	"github.com/jpillora/go-echo-server/handler"
	"github.com/jpillora/opts"
)

var VERSION string = "0.0.0-src"

func main() {
	c := struct {
		Port        int `help:"Port" env:"PORT"`
		echo.Config `type:"embedded"`
	}{
		Port: 3000,
	}
	opts.New(&c).
		Name("go-echo-server").
		Version(VERSION).
		Repo("github.com/jpillora/go-echo-server").
		Parse()
	h := echo.New(c.Config)
	log.Printf("Listening on %d...", c.Port)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(c.Port), h))
}
