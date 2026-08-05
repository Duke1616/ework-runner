package jwt

import (
	"context"

	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/golang-jwt/jwt/v4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ClientInterceptorBuilder 客户端拦截器构建器
type ClientInterceptorBuilder struct {
	jwtKey string
}

// NewClientInterceptorBuilder 创建客户端拦截器构建器
func NewClientInterceptorBuilder(jwtKey string) *ClientInterceptorBuilder {
	return &ClientInterceptorBuilder{
		jwtKey: jwtKey,
	}
}

// UnaryClientInterceptor 创建一元客户端拦截器
func (b *ClientInterceptorBuilder) UnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(b.withJWTContext(ctx), method, req, reply, cc, opts...)
	}
}

// StreamClientInterceptor 为流式请求注入与一元请求相同的 JWT metadata。
func (b *ClientInterceptorBuilder) StreamClientInterceptor() grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn,
		method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return streamer(b.withJWTContext(ctx), desc, cc, method, opts...)
	}
}

func (b *ClientInterceptorBuilder) withJWTContext(ctx context.Context) context.Context {
	if b.hasJWTInContext(ctx) {
		return ctx
	}
	return b.injectJWTContext(ctx)
}

// hasJWTInContext 检查 context 中是否已经有 JWT 信息
func (b *ClientInterceptorBuilder) hasJWTInContext(ctx context.Context) bool {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return false
	}

	authHeaders := md.Get(AuthorizationKey)
	return len(authHeaders) > 0
}

// injectJWTContext 注入 jwt context
func (b *ClientInterceptorBuilder) injectJWTContext(ctx context.Context) context.Context {
	jwtAuth := NewJwtAuth(b.jwtKey)

	// 自动将当前上下文的租户 ID 和用户 ID 注入到服务间自签发的 JWT Claims 中，实现透明透传
	claims := jwt.MapClaims{}
	if tid := ctxutil.GetTenantID(ctx); tid > 0 {
		claims["tenant_id"] = tid
	}
	if uid := ctxutil.GetUserID(ctx); uid > 0 {
		claims["user_id"] = uid
	}

	tokenString, err := jwtAuth.Encode(claims)
	if err != nil {
		return ctx
	}

	// 追加， 不可以是覆盖
	return metadata.AppendToOutgoingContext(ctx, AuthorizationKey, BearerPrefix+tokenString)
}
