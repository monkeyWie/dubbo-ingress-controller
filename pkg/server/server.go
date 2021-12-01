package server

import (
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
			requestData, err := resolveRequest(clientConn)
			if err != nil {
				logger.Errorf("resolve request error:%v", err)
				return
			}
			if err := s.handleData(clientConn, &proxyConn, requestData); err != nil {
				logger.Warnf("handle data error:%v", err)
				return
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

	if _, err = (*proxyConn).Write(data); err != nil {
		return errors.WithStack(err)
	}
	if _, err := io.Copy(*proxyConn, clientConn); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func resolveRequest(reader io.Reader) ([]byte, error) {
	headerData := make([]byte, impl.HEADER_LENGTH)
	if _, err := io.ReadFull(reader, headerData); err != nil {
		return nil, errors.WithStack(err)
	}

	headerBuf := bytes.NewBuffer(headerData)
	pkg := impl.NewDubboPackage(headerBuf)
	if err := pkg.ReadHeader(); err != nil && !errors.Is(err, hessian.ErrBodyNotEnough) {
		return nil, errors.WithStack(err)
	}

	if !pkg.IsRequest() {
		return nil, errors.New("not request")
	}

	bodyData := make([]byte, pkg.GetBodyLen())
	if _, err := io.ReadFull(reader, bodyData); err != nil {
		return nil, errors.WithStack(err)
	}

	return append(headerData, bodyData...), nil
}
