package jwt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/golang-jwt/jwt/v4"
)

const (
	AuthorizationKey  = "Authorization"
	BearerPrefix      = "Bearer "
	DefaultIssuer     = "ework-runner"
	DefaultExpiration = 24 * time.Hour
)

type InterceptorBuilder struct {
	key    string
	issuer string
	exp    time.Duration
}

func (b *InterceptorBuilder) Decode(tokenString string) (jwt.MapClaims, error) {
	// 去除可能的 Bearer 前缀（兼容不同客户端实现）
	tokenString = strings.TrimPrefix(tokenString, BearerPrefix)

	// 解析 Token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("不支持的签名算法: %v", token.Header["alg"])
		}
		return []byte(b.key), nil
	})
	// 错误处理
	if err != nil {
		return nil, fmt.Errorf("令牌解析失败: %w", err)
	}

	// 验证 Token 有效性
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, fmt.Errorf("无效的令牌")
}

// Encode 生成 JWT Token，支持自定义声明和自动添加标准声明
func (b *InterceptorBuilder) Encode(customClaims jwt.MapClaims) (string, error) {
	// 合并自定义声明和默认声明
	claims := jwt.MapClaims{
		"iat": time.Now().Unix(),
		"iss": b.issuer,
	}

	// 合并用户自定义声明（覆盖默认声明）
	for k, v := range customClaims {
		claims[k] = v
	}

	// 自动处理过期时间
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(b.exp).Unix()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(b.key))
}

func (b *InterceptorBuilder) JwtAuthInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		ctx, err := b.authenticate(ctx)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// JwtAuthStreamInterceptor 验证流式请求，并将受信 claims 注入 ServerStream context。
func (b *InterceptorBuilder) JwtAuthStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, _ *grpc.StreamServerInfo,
		handler grpc.StreamHandler) error {
		ctx, err := b.authenticate(stream.Context())
		if err != nil {
			return err
		}
		return handler(srv, &authenticatedServerStream{ServerStream: stream, ctx: ctx})
	}
}

func (b *InterceptorBuilder) authenticate(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "请求缺少元数据 metadata")
	}
	authHeaders := md.Get(AuthorizationKey)
	if len(authHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, "请求缺少授权令牌 Authorization")
	}
	claims, err := b.Decode(authHeaders[0])
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, status.Error(codes.Unauthenticated, "授权令牌已过期")
		}
		if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return nil, status.Error(codes.Unauthenticated, "授权令牌签名无效")
		}
		return nil, status.Error(codes.Unauthenticated, "无效的授权令牌: "+err.Error())
	}
	return Set(ctx, claims), nil
}

type authenticatedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authenticatedServerStream) Context() context.Context { return s.ctx }

// Option 配置选项
type Option func(*InterceptorBuilder)

// WithIssuer 设置签发者
func WithIssuer(issuer string) Option {
	return func(b *InterceptorBuilder) {
		b.issuer = issuer
	}
}

// WithExpiration 设置过期时间
func WithExpiration(exp time.Duration) Option {
	return func(b *InterceptorBuilder) {
		b.exp = exp
	}
}

// NewJwtAuth 创建 JWT 认证拦截器
func NewJwtAuth(key string, opts ...Option) *InterceptorBuilder {
	b := &InterceptorBuilder{
		key:    key,
		issuer: DefaultIssuer,
		exp:    DefaultExpiration,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}
