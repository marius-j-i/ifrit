package udp

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/joonnna/ifrit/log"
	"github.com/joonnna/ifrit/netutil"
)

type Server struct {
	conn *net.UDPConn
	addr string
}

func NewServer() (*Server, error) {
	port := netutil.GetOpenPort()
	//hostName := netutil.GetLocalIP()
	hostName, _ := os.Hostname()
	addr := fmt.Sprintf("%s:%d", hostName, port)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	addrs, err := net.LookupHost(hostName)
	if err != nil {
		return nil, err
	}

	externalAddr := fmt.Sprintf("%s:%d", addrs[0], udpAddr.Port)

	return &Server{conn: conn, addr: externalAddr}, nil
}

func (s Server) Send(addr string, data []byte) ([]byte, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	c, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	c.SetDeadline(time.Now().Add(time.Second * 5))
	defer c.Close()

	_, err = c.Write(data)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	bytes := make([]byte, 256)

	n, err := c.Read(bytes)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}

	return bytes[:n], nil
}

func (s *Server) Serve(signMsg func([]byte) ([]byte, error), exitChan chan bool) error {
	bytes := make([]byte, 256)
	for {
		select {
		case <-exitChan:
			return nil
		default:
			n, addr, err := s.conn.ReadFrom(bytes)
			if err != nil {
				log.Error(err.Error())
				continue
			}

			resp, err := signMsg(bytes[:n])
			if err != nil {
				log.Error(err.Error())
				continue

			}

			s.conn.SetWriteDeadline(time.Now().Add(time.Second * 3))
			_, err = s.conn.WriteTo(resp, addr)
			if err != nil {
				log.Error(err.Error())
				continue
			}
		}
	}

	return nil
}

func (s Server) Addr() string {
	return s.addr
}

func (s *Server) Shutdown() {
	s.conn.Close()
}