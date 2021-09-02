package main

import (
	"fmt"
	"github.com/itsabgr/go-handy"
	core "github.com/itsabgr/ilam/pkg/core"
	"github.com/kkyr/fig"
	"log"
	"os"
	"os/signal"
	"path/filepath"
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
func getConfigPath() (string, string) {
	path := os.Environ()[len(os.Environ())-1]
	file, err := os.OpenFile(path, os.O_RDONLY, 0666)
	handy.Close(file)
	if err != nil {
		if os.IsNotExist(err) {
			path = "./config.yaml"
		} else {
			panic(err)
		}
	}
	path, err = filepath.Abs(path)
	handy.Throw(err)
	return filepath.Dir(path), filepath.Base(path)

}
func main() {
	defer handy.Catch(func(recovered interface{}) {
		logger.Fatal(recovered)
	})
	configFileDir, configFileName := getConfigPath()
	logger.Println("Version:", core.Version)
	logger.Printf("Open: %s/%s\n", configFileDir, configFileName)

	handy.Throw(
		fig.Load(&CONFIG,
			fig.File(configFileName),
			fig.Dirs(configFileDir),
		),
	)
	server := core.New(core.Config{
		Logger: logger,
		Addr:   CONFIG.ADDR,
		Origin: CONFIG.ORIGIN,
	})
	defer handy.Close(server)
	logger.Println(fmt.Sprintf("Config %+v", CONFIG))
	handy.Throw(server.Listen(CONFIG.CERT, CONFIG.KEY))
}
