package etcd

import (
	"strings"
	"testing"
)

func TestServiceKeyIsDelimiterSafe(t *testing.T) {
	r := &Registry{prefix: DefaultPrefix}
	prefix := r.serviceKey("foo")
	if prefix != "/grpc/services/foo/" {
		t.Fatalf("service key = %q", prefix)
	}
	if strings.HasPrefix("/grpc/services/foobar/127.0.0.1:80", prefix) {
		t.Fatal("foo service prefix must not match foobar")
	}
}
