package main

import (
	"context"
	"dubbo.apache.org/dubbo-go/v3/common/logger"
	"github/monkeyWie/dubbo-ingress/pkg/server"
	"github/monkeyWie/dubbo-ingress/pkg/watcher"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path"
)

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Fatal(err)
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", path.Join(homeDir, "/.kube/config"))
	if err != nil {
		logger.Fatal(err)
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logger.Fatal(err)
	}

	server := server.NewServer()
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
	watcher := watcher.NewWatcher(client, eventHandler)

	eg, ctx := errgroup.WithContext(context.TODO())
	eg.Go(func() error {
		return server.Start(ctx)
	})
	eg.Go(func() error {
		return watcher.Start(ctx)
	})
	if err := eg.Wait(); err != nil {
		logger.Fatal(err)
	}
}
