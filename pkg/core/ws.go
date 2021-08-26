package core

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/itsabgr/go-handy"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server interface {
	Listen(certPath, keyPath string) error
	Statics() Statics
	io.Closer
}
type server struct {
	http         http.Server
	upgrader     websocket.Upgrader
	conf         *Config
	connMapMutex sync.RWMutex
	connMap      map[ID]*websocket.Conn
}

type Config struct {
	Addr          string
	Logger        *log.Logger
	Origin        string
	LaunchAt      time.Time
	Authenticator func(Server, *http.Request, ID) error
}

var zeroTime = time.Time{}

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
	if s.conf.LaunchAt == zeroTime {
		s.conf.LaunchAt = time.Now()
	}
	s.upgrader.CheckOrigin = func(r *http.Request) bool {
		if len(s.conf.Origin) == 0 {
			return true
		}
		return s.conf.Origin == r.Header.Get("Origin")
	}
	s.connMap = make(map[ID]*websocket.Conn)
	return s
}
func (s *server) Listen(certPath, keyPath string) error {
	return s.http.ListenAndServeTLS(certPath, keyPath)
}
func parseId(r *http.Request) (ID, error) {
	path := strings.Replace(r.URL.Path, "/", "", 1)
	id, err := strconv.Atoi(path)
	return ID(id), err
}
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		switch {
		case r.URL.Path == "/":
			s.routeStatics(w, r)
		default:
			s.routeUpgrade(w, r)
		}
	case http.MethodOptions:
		s.routeCors(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
func (s *server) mapAdd(cid ID, conn *websocket.Conn) error {
	s.connMapMutex.Lock()
	defer s.connMapMutex.Unlock()
	if _, exists := s.connMap[cid]; exists {
		return os.ErrExist
	}
	s.connMap[cid] = conn
	return nil
}
func (s *server) mapSet(cid ID, conn *websocket.Conn) error {
	s.connMapMutex.Lock()
	defer s.connMapMutex.Unlock()
	s.connMap[cid] = conn
	return nil
}
func (s *server) mapDelete(cid ID) {
	s.connMapMutex.Lock()
	defer s.connMapMutex.Unlock()
	delete(s.connMap, cid)
}
func (s *server) mapGet(cid ID) (*websocket.Conn, error) {
	s.connMapMutex.RLock()
	defer s.connMapMutex.RUnlock()
	conn, exists := s.connMap[cid]
	if !exists {
		return nil, os.ErrNotExist
	}
	return conn, nil
}
func (s *server) routeUpgrade(w http.ResponseWriter, r *http.Request) {
	if s.mapLen() >= math.MaxInt32 {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	cid, err := parseId(r)
	if err != nil {
		s.conf.Logger.Printf("cid: error: %s\n", err.Error())
		return
	}
	err = s.conf.Authenticator(s, r, cid)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(fmt.Sprintf(`{"error":"%s"}`, err.Error())))
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.conf.Logger.Printf("upgrade: error: %s\n", err.Error())
		return
	}
	defer conn.Close()
	defer s.mapDelete(cid)
	err = s.mapAdd(cid, conn)
	if err != nil {
		s.conf.Logger.Printf("cid: add: error: %s\n", err.Error())
		return
	}
	for {
		kind, message, err := conn.ReadMessage()
		if err != nil {
			s.conf.Logger.Printf("ws: error: %s\n", err.Error())
			break
		}
		switch kind {
		case websocket.CloseMessage:
			return
		case websocket.PingMessage:
		case websocket.PongMessage:
			s.mapSet(cid, conn)
		case websocket.TextMessage:
			conn.WriteMessage(websocket.CloseMessage, []byte{})
			s.conf.Logger.Printf("ws: msg: error: received text message")
			return
		case websocket.BinaryMessage:
			err = s.handleMessage(cid, message)
			if err != nil {
				s.conf.Logger.Printf("ws: msg: error: %s\n", err.Error())
				return
			}
		}

	}
}
func isAllZero(bytes []byte) bool {
	for _, b := range bytes {
		if b != 0 {
			return false
		}
	}
	return true
}

type ID = uint64

func (s *server) handleMessage(_ ID, data []byte) error {
	message, err := decodeIncoming(data)
	if err != nil {
		return err
	}
	switch {
	case isAllZero(message.ip) && message.port == 0:
		conn, err := s.mapGet(message.id)
		if err != nil {
			return err
		}
		conn.WriteMessage(websocket.BinaryMessage, message.payload)
		return nil
	}
	return errors.New("unsupported message")
}

type incoming struct {
	ip      []byte
	port    int
	id      ID
	payload []byte
}

const incomingMagic byte = 1

type incomingFlag byte

const (
	flagSendToIpv4 = iota + 1
)

func decodeIncoming(data []byte) (*incoming, error) {
	p := incoming{}
	if incomingMagic != data[0] {
		return nil, errors.New("invalid magic number")
	}
	flag := data[1]
	switch flag {
	case flagSendToIpv4:
		p.ip = data[2 : 2+4]
		p.port = int(binary.BigEndian.Uint16(data[2+4 : 2+4+2]))
		p.id = binary.BigEndian.Uint64(data[2+4+2 : 2+4+2+8])
		p.payload = data[2+4+2+8:]
	default:
		return nil, errors.New("unsupported flag")
	}
	return &p, nil
}
func (s *server) routeCors(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", s.conf.Origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.WriteHeader(http.StatusNoContent)

}
func (s *server) routeStatics(w http.ResponseWriter, r *http.Request) {
	response, err := json.Marshal(s.Statics())
	fmt.Println(2)
	handy.Throw(err)
	w.Write(response)
}
func (s *server) mapLen() int {
	s.connMapMutex.RLock()
	defer s.connMapMutex.RUnlock()
	return len(s.connMap)
}

type Statics struct {
	LaunchTime  string
	NowTime     string
	Connections int32
}

func (s *server) Statics() Statics {
	statics := Statics{}
	statics.Connections = int32(s.mapLen())
	statics.LaunchTime = strconv.FormatInt(s.conf.LaunchAt.UTC().Unix(), 10)
	statics.NowTime = strconv.FormatInt(time.Now().UTC().Unix(), 10)
	return statics
}

func (s *server) Close() error {
	return s.http.Close()
}

func main() {

}
