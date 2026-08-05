package interceptors

import (
	"github.com/Duke1616/etask/pkg/grpc/interceptors/bizid"
	jwtinterceptor "github.com/Duke1616/etask/pkg/grpc/interceptors/jwt"
	"github.com/Duke1616/etask/pkg/grpc/interceptors/tenant"
	"google.golang.org/grpc"
)

// ServerPipeline 服务端拦截器管道拼装门面
type ServerPipeline struct {
	authToken string
}

// NewServerPipeline 创建服务端拦截器管道
func NewServerPipeline(authToken string) *ServerPipeline {
	return &ServerPipeline{authToken: authToken}
}

// Build 拼装并输出有序的服务端拦截器链 (一元链 与 流式链)
func (p *ServerPipeline) Build() ([]grpc.UnaryServerInterceptor, []grpc.StreamServerInterceptor) {
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		bizid.UnaryServerInterceptor(),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		bizid.StreamServerInterceptor(),
	}

	// 配置了认证令牌则启用 JWT 验证防线，否则在内网环境下降级为明文租户透传
	if p.authToken != "" {
		jwtAuth := jwtinterceptor.NewJwtAuth(p.authToken)
		unaryInterceptors = append(unaryInterceptors, jwtAuth.JwtAuthInterceptor())
		streamInterceptors = append(streamInterceptors, jwtAuth.JwtAuthStreamInterceptor())
	} else {
		unaryInterceptors = append(unaryInterceptors, tenant.UnaryServerInterceptor())
		streamInterceptors = append(streamInterceptors, tenant.StreamServerInterceptor())
	}

	return unaryInterceptors, streamInterceptors
}

// ClientPipeline 客户端拦截器管道拼装门面
type ClientPipeline struct {
	authToken string
}

// NewClientPipeline 创建客户端拦截器管道
func NewClientPipeline(authToken string) *ClientPipeline {
	return &ClientPipeline{authToken: authToken}
}

// Build 拼装并输出有序的客户端拦截器链 (一元链 与 流式链)
func (p *ClientPipeline) Build() ([]grpc.UnaryClientInterceptor, []grpc.StreamClientInterceptor) {
	unaryInterceptors := []grpc.UnaryClientInterceptor{
		bizid.UnaryClientInterceptor(),
	}
	streamInterceptors := []grpc.StreamClientInterceptor{
		bizid.StreamClientInterceptor(),
	}

	// 启用安全认证则通过自签发 JWT 承载身份，否则在内网环境下通过 Metadata 头部透传
	if p.authToken != "" {
		jwtInterceptor := jwtinterceptor.NewClientInterceptorBuilder(p.authToken)
		unaryInterceptors = append(unaryInterceptors, jwtInterceptor.UnaryClientInterceptor())
		streamInterceptors = append(streamInterceptors, jwtInterceptor.StreamClientInterceptor())
	} else {
		unaryInterceptors = append(unaryInterceptors, tenant.UnaryClientInterceptor())
		streamInterceptors = append(streamInterceptors, tenant.StreamClientInterceptor())
	}

	return unaryInterceptors, streamInterceptors
}
