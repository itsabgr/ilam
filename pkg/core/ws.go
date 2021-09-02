package core

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/itsabgr/fastintmap"
	"github.com/itsabgr/go-handy"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const Version = "0.1.0"

//Server represents message broker
type Server interface {
	Listen(certPath, keyPath string) error
	Statics() Statics
	io.Closer
}
type server struct {
	_noCopy    handy.NoCopy
	http       http.Server
	upgrader   websocket.Upgrader
	conf       *Config
	connMap    fastintmap.HashMap
	launchTime time.Time
}

//Config is server config params
type Config struct {
	Addr          string
	Logger        *log.Logger
	Origin        string
	Authenticator func(Server, *http.Request, ID) error
}

//New make a new Server with conf as its Config
func New(conf Config) Server {
	s := &server{}
	s.http.Addr = conf.Addr
	s.http.ErrorLog = conf.Logger
	s.http.Handler = s
	s.http.SetKeepAlivesEnabled(false)
	s.conf = &conf
	if s.conf.Authenticator == nil {
		s.conf.Authenticator = func(_ Server, _ *http.Request, _ ID) error {
			return nil
		}
	}

	s.upgrader.CheckOrigin = func(r *http.Request) bool {
		switch s.conf.Origin {
		case "", "*":
			return true
		}
		return s.conf.Origin == r.Header.Get("Origin")
	}
	//s.connMap = make(map[ID]*websocket.Conn)
	s.launchTime = time.Now().UTC()
	return s
}
func (s *server) Listen(certPath, keyPath string) error {
	if len(certPath) == 0 && len(keyPath) == 0 {
		return s.http.ListenAndServe()
	}
	return s.http.ListenAndServeTLS(certPath, keyPath)
}

func parseID(r *http.Request) (ID, error) {
	path := strings.Replace(r.URL.Path, "/", "", 1)
	id, err := strconv.ParseUint(path, 10, 64)
	return id, err
}
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Server", "ilam/0.1.0")
	switch r.Method {
	case http.MethodGet:
		switch {
		case r.URL.Path == "/":
			s.routeStatics(w, r)
		default:
			s.routeUpgrade(w, r)
		}
	case http.MethodPost:
		s.routePost(w, r)
	case http.MethodOptions:
		s.routeCors(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
func (s *server) mapAdd(cid ID, conn *websocket.Conn) error {
	if !s.connMap.Cas(uintptr(cid), nil, conn) {
		return os.ErrExist
	}
	return nil
}
func (s *server) mapSet(cid ID, conn *websocket.Conn) error {
	s.connMap.Set(uintptr(cid), conn)
	return nil
}
func (s *server) mapDelete(cid ID) {
	s.connMap.Del(uintptr(cid))
}
func (s *server) mapGet(cid ID) (*websocket.Conn, error) {
	conn, exists := s.connMap.Get(uintptr(cid))
	if !exists {
		return nil, os.ErrNotExist
	}
	return conn.(*websocket.Conn), nil
}
func (s *server) routePost(w http.ResponseWriter, r *http.Request) {
	cid, err := parseID(r)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
		s.conf.Logger.Printf("cid: error: %s\n", err.Error())
		return
	}
	conn, err := s.mapGet(cid)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
		s.conf.Logger.Printf("cid: error: %s\n", err.Error())
		return
	}
	err = s.conf.Authenticator(s, r, cid)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
		s.conf.Logger.Printf("auth: error: %s\n", err.Error())
		return
	}
	message, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
		s.conf.Logger.Printf("body: error: %s\n", err.Error())
		return
	}
	err = conn.WriteMessage(websocket.BinaryMessage, message)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
		s.conf.Logger.Printf("send: error: %s\n", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) routeUpgrade(w http.ResponseWriter, r *http.Request) {
	if s.mapLen() >= math.MaxInt32 {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	cid, err := parseID(r)
	if err != nil {
		s.conf.Logger.Printf("cid: error: %s\n", err.Error())
		return
	}
	err = s.conf.Authenticator(s, r, cid)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.conf.Logger.Printf("upgrade: error: %s\n", err.Error())
		return
	}
	defer Close(conn)
	defer s.mapDelete(cid)
	err = s.mapAdd(cid, conn)
	if err != nil {
		s.conf.Logger.Printf("cid: add: error: %s\n", err.Error())
		return
	}
	for {
		kind, _, err := conn.ReadMessage()
		if err != nil {
			s.conf.Logger.Printf("ws: error: %s\n", err.Error())
			return
		}
		switch kind {
		case websocket.CloseMessage:
			return
		case websocket.PingMessage:
		case websocket.PongMessage:
			err = s.mapSet(cid, conn)
			if err != nil {
				s.conf.Logger.Printf("map: set: %s\n", err.Error())
				return
			}
		case websocket.TextMessage, websocket.BinaryMessage:
			s.conf.Logger.Printf("ws: msg: error: received a message")
			return
		}
	}
}

//ID represents a node ID
type ID = uint64

func (s *server) routeCors(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", s.conf.Origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.WriteHeader(http.StatusNoContent)

}
func (s *server) routeStatics(w http.ResponseWriter, _ *http.Request) {
	response, err := json.Marshal(s.Statics())
	fmt.Println(2)
	handy.Throw(err)
	_, _ = w.Write(response)
}
func (s *server) mapLen() int {
	return s.connMap.Len()
}

//Statics is Server status and Statics
type Statics struct {
	UpTime      int32
	Connections int32
	Version     string
}

func (s *server) Statics() Statics {
	statics := Statics{}
	statics.Connections = int32(s.mapLen())
	statics.UpTime = int32(time.Now().UTC().Unix() - s.launchTime.Unix())
	statics.Version = Version
	return statics
}

//Close do server force close
func (s *server) Close() error {
	return s.http.Close()
}

//Close closes a io.Closer and ignores its error
func Close(closer io.Closer) {
	_ = closer.Close()
}
