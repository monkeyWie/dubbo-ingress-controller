package controller

import (
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"path"
	"testing"
)

func TestController_Start(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", path.Join(homeDir, "/.kube/config"))
	if err != nil {
		t.Fatal(err)
	}
	controller, err := NewController(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
}
