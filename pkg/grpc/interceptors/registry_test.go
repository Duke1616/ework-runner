package interceptors

import "testing"

func TestPipelinesIncludeAuthOnUnaryAndStream(t *testing.T) {
	serverUnary, serverStream := NewServerPipeline("secret").Build()
	if len(serverUnary) != 2 || len(serverStream) != 2 {
		t.Fatalf("server pipeline lengths = (%d, %d), want (2, 2)", len(serverUnary), len(serverStream))
	}

	clientUnary, clientStream := NewClientPipeline("secret").Build()
	if len(clientUnary) != 2 || len(clientStream) != 2 {
		t.Fatalf("client pipeline lengths = (%d, %d), want (2, 2)", len(clientUnary), len(clientStream))
	}
}
