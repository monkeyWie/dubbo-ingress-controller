package server

import (
	"context"
	"testing"
)

func TestServer_Start(t *testing.T) {
	server := NewServer(20881)
	server.PutRoute("sms-rpc", "127.0.0.1:20880")
	if err := server.Start(context.TODO()); err != nil {
		t.Fatal(err)
	}
}
