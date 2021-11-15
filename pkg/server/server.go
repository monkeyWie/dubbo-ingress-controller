package server

import (
	"bufio"
	"bytes"
	"context"
	"dubbo.apache.org/dubbo-go/v3/common/logger"
	"dubbo.apache.org/dubbo-go/v3/protocol/dubbo/impl"
	"fmt"
	hessian "github.com/apache/dubbo-go-hessian2"
	"github.com/pkg/errors"
	"io"
	"net"
	"sync"
)

type Server struct {
	port         int
	routingTable *sync.Map
}

func NewServer(port int) *Server {
	return &Server{
		port:         port,
		routingTable: &sync.Map{},
	}
}

func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return errors.WithStack(err)
	}
	for {
		clientConn, err := listener.Accept()
		if err != nil {
			logger.Errorf("accept error:%v", err)
			continue
		}
		go func() {
			defer clientConn.Close()
			var proxyConn net.Conn
			defer func() {
				if proxyConn != nil {
					proxyConn.Close()
				}
			}()
			scanner := bufio.NewScanner(clientConn)
			scanner.Split(split)
			for scanner.Scan() {
				if err := s.handleData(clientConn, &proxyConn, scanner.Bytes()); err != nil {
					logger.Warnf("handle data error:%v", err)
					return
				}
			}
			/*if err := scanner.Err(); err != nil {
				logger.Warnf("scanner error:%v", err)
				return
			}*/
		}()
	}
}

func (s *Server) PutRoute(target string, host string) {
	s.routingTable.Store(target, host)
	s.logRoute()
}

func (s *Server) DeleteRoute(target string) {
	s.routingTable.Delete(target)
	s.logRoute()
}

func (s *Server) logRoute() {
	str := ""
	s.routingTable.Range(func(key, value interface{}) bool {
		str += fmt.Sprintf("%s=%s\t", key, value)
		return true
	})
	logger.Infof("routing table: %s", str)
}

func (s *Server) handleData(clientConn net.Conn, proxyConn *net.Conn, data []byte) (err error) {
	buf := bytes.NewBuffer(data)
	pkg := impl.NewDubboPackage(buf)
	if err = pkg.Unmarshal(); err != nil {
		return errors.WithStack(err)
	}
	body, ok := pkg.Body.(map[string]interface{})
	if !ok {
		return errors.New("bad body")
	}
	attachments, ok := body["attachments"]
	if !ok {
		return errors.New("no attachments")
	}
	attrs, ok := attachments.(map[string]interface{})
	if !ok {
		return errors.New("bad attachments")
	}
	target, ok := attrs["target-application"]
	if !ok {
		return errors.New("no target-application")
	}
	if *proxyConn == nil {
		logger.Infof("target application: %s", target)
		host, ok := s.routingTable.Load(target)
		if !ok {
			return errors.Errorf("no routing: %s", target)
		}
		*proxyConn, err = net.Dial("tcp", host.(string))
		if err != nil {
			return errors.WithStack(err)
		}
		logger.Infof("target connected:[%s] %s", target, host)
		go func() {
			io.Copy(clientConn, *proxyConn)
		}()
	}
	if _, err = (*proxyConn).Write(data); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func split(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	buf := bytes.NewBuffer(data)
	pkg := impl.NewDubboPackage(buf)
	err = pkg.ReadHeader()
	if err != nil {
		if errors.Is(err, hessian.ErrHeaderNotEnough) || errors.Is(err, hessian.ErrBodyNotEnough) {
			return 0, nil, nil
		}
		return 0, nil, err
	}
	if !pkg.IsRequest() {
		return 0, nil, errors.New("not request")
	}
	requestLen := impl.HEADER_LENGTH + pkg.Header.BodyLen
	if len(data) < requestLen {
		return 0, nil, nil
	}
	return requestLen, data[0:requestLen], nil
}
