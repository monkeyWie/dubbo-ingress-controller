package controller

import (
	"context"
	"dubbo.apache.org/dubbo-go/v3/common/logger"
	"github/monkeyWie/dubbo-ingress-controller/pkg/server"
	"github/monkeyWie/dubbo-ingress-controller/pkg/watcher"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
	"strconv"
)

type Controller struct {
	client *kubernetes.Clientset
}

func NewController(cfg *rest.Config) (*Controller, error) {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Controller{client: client}, nil
}

func (c *Controller) Start() error {
	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = "20880"
	}
	port, err := strconv.Atoi(serverPort)
	if err != nil {
		logger.Fatal(err)
	}

	server := server.NewServer(port)
	eventHandler := watcher.EventHandler{
		OnAdd: func(data *watcher.EventData) {
			server.PutRoute(data.Host, data.Service)
		},
		OnUpdate: func(oldData *watcher.EventData, newData *watcher.EventData) {
			server.PutRoute(newData.Host, newData.Service)
			if oldData != nil && oldData.Host != newData.Host {
				server.DeleteRoute(oldData.Host)
			}
		},
		OnDelete: func(data *watcher.EventData) {
			server.DeleteRoute(data.Host)
		},
	}
	watcher := watcher.NewWatcher(c.client, eventHandler)

	eg, ctx := errgroup.WithContext(context.TODO())
	eg.Go(func() error {
		return server.Start(ctx)
	})
	eg.Go(func() error {
		return watcher.Start(ctx)
	})
	if err := eg.Wait(); err != nil {
		return err
	}
	return nil
}
