package jwt

import (
	"context"
	"testing"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	jwtlib "github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s contextServerStream) Context() context.Context { return s.ctx }

func TestJwtAuthStreamInterceptor(t *testing.T) {
	auth := NewJwtAuth("stream-secret")
	token, err := auth.Encode(jwtlib.MapClaims{"tenant_id": int64(42)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(
		AuthorizationKey, BearerPrefix+token,
	))

	called := false
	err = auth.JwtAuthStreamInterceptor()(nil, contextServerStream{ctx: ctx}, nil,
		func(_ interface{}, stream grpc.ServerStream) error {
			called = true
			if tenantID := ctxutil.GetTenantID(stream.Context()).Int64(); tenantID != 42 {
				t.Fatalf("tenant ID = %d, want 42", tenantID)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("stream interceptor returned error: %v", err)
	}
	if !called {
		t.Fatal("stream handler was not called")
	}
}

func TestJwtAuthStreamInterceptorRejectsMissingToken(t *testing.T) {
	auth := NewJwtAuth("stream-secret")
	err := auth.JwtAuthStreamInterceptor()(nil, contextServerStream{ctx: t.Context()}, nil,
		func(interface{}, grpc.ServerStream) error {
			t.Fatal("stream handler must not be called")
			return nil
		})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("error code = %s, want %s", status.Code(err), codes.Unauthenticated)
	}
}

func TestJWTStreamClientInterceptorAddsAuthorization(t *testing.T) {
	builder := NewClientInterceptorBuilder("stream-secret")
	ctx := ctxutil.WithTenantID(t.Context(), 42)

	_, err := builder.StreamClientInterceptor()(ctx, &grpc.StreamDesc{}, nil, "/test.Stream",
		func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string,
			_ ...grpc.CallOption) (grpc.ClientStream, error) {
			md, ok := metadata.FromOutgoingContext(ctx)
			if !ok || len(md.Get(AuthorizationKey)) != 1 {
				t.Fatalf("authorization metadata missing: %v", md)
			}
			return nil, nil
		})
	if err != nil {
		t.Fatalf("stream client interceptor returned error: %v", err)
	}
}
