package main

import (
	"fmt"
	"github.com/itsabgr/go-handy"
	core "github.com/itsabgr/ilam/pkg/core"
	"github.com/kkyr/fig"
	"log"
	"os"
	"os/signal"
)

//CONFIG is app config
var CONFIG struct {
	ADDR   string `fig:"ADDR" default:"localhost:4433"`
	CERT   string `fig:"CERT"`
	KEY    string `fig:"KEY"`
	ORIGIN string `fig:"ORIGIN"`
}
var logger = log.New(os.Stdout, "", 0)

func init() {
	var closeSignal = make(chan os.Signal)
	signal.Notify(closeSignal, os.Interrupt, os.Kill)
	go func() {
		logger.Fatal("signal: ", <-closeSignal)
	}()
}
func main() {
	defer handy.Catch(func(recovered interface{}) {
		logger.Fatal(recovered)
	})

	handy.Throw(
		fig.Load(&CONFIG,
			fig.File("config.yaml"),
			fig.Dirs(os.Environ()[len(os.Environ())-1], "."),
		),
	)
	server := core.New(core.Config{
		Logger: logger,
		Addr:   CONFIG.ADDR,
		Origin: CONFIG.ORIGIN,
	})
	defer handy.Close(server)
	logger.Println(fmt.Sprintf("%+v", CONFIG))
	handy.Throw(server.Listen(CONFIG.CERT, CONFIG.KEY))
}
