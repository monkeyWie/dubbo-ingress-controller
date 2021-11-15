package main

import (
	"dubbo.apache.org/dubbo-go/v3/common/logger"
	"github/monkeyWie/dubbo-ingress-controller/pkg/controller"
	"k8s.io/client-go/rest"
)

func main() {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		logger.Fatal(err)
	}

	controller, err := controller.NewController(cfg)
	if err != nil {
		logger.Fatal(err)
	}
	if err := controller.Start(); err != nil {
		logger.Fatal(err)
	}
}
