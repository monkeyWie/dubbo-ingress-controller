package server

import (
	"context"
	"testing"
)

func TestServer_Start(t *testing.T) {
	server := NewServer()
	server.PutRoute("hello-dubbo-provider", "127.0.0.1:20880")
	if err := server.Start(context.TODO()); err != nil {
		t.Fatal(err)
	}
}
