package server

import (
	"bufio"
	"bytes"
	"context"
	"dubbo.apache.org/dubbo-go/v3/common/logger"
	"dubbo.apache.org/dubbo-go/v3/protocol/dubbo/impl"
	hessian "github.com/apache/dubbo-go-hessian2"
	"github.com/pkg/errors"
	"io"
	"net"
	"sync"
)

type Server struct {
	routingTable *sync.Map
}

func NewServer() *Server {
	return &Server{
		routingTable: &sync.Map{},
	}
}

func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", ":20881")
	if err != nil {
		return errors.WithStack(err)
	}
	for {
		clientConn, err := listener.Accept()
		if err != nil {
			logger.Errorf("accept error:%v", err)
			continue
		}
		logger.Info("client accept")
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
				if err := s.handleData(clientConn, proxyConn, scanner.Bytes()); err != nil {
					logger.Warnf("handle data error:%v", err)
					return
				}
			}
			if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
				logger.Warnf("scanner error:%v", err)
				return
			}

		}()
	}
}

func (s *Server) PutRoute(target string, host string) {
	s.routingTable.Store(target, host)
}

func (s *Server) DeleteRoute(target string) {
	s.routingTable.Delete(target)
}

func (s *Server) handleData(clientConn net.Conn, proxyConn net.Conn, data []byte) (err error) {
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
	if proxyConn == nil {
		host, _ := s.routingTable.Load(target)
		proxyConn, err = net.Dial("tcp", host.(string))
		if err != nil {
			return errors.WithStack(err)
		}
		logger.Infof("target connected:[%s] %s", target, host)
		go func() {
			if _, err2 := io.Copy(clientConn, proxyConn); err2 != nil && !errors.Is(err2, io.EOF) {
				logger.Errorf("io copy error: %v", err2)
			}
		}()
	}
	if _, err = proxyConn.Write(data); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func split(data []byte, atEOF bool) (advance int, token []byte, err error) {
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
