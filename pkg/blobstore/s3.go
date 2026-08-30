package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/s3utils"
	"github.com/samber/lo"
)

type S3Config struct {
	Endpoint     string `mapstructure:"endpoint" yaml:"endpoint"`
	Secure       bool   `mapstructure:"secure" yaml:"secure"`
	Region       string `mapstructure:"region" yaml:"region"`
	Bucket       string `mapstructure:"bucket" yaml:"bucket"`
	Prefix       string `mapstructure:"prefix" yaml:"prefix"`
	AccessKey    string `mapstructure:"access_key" yaml:"access_key"`
	SecretKey    string `mapstructure:"secret_key" yaml:"secret_key"`
	SessionToken string `mapstructure:"session_token" yaml:"session_token"`
}

type S3 struct {
	client *minio.Client
	bucket string
	prefix string
}

func NewS3(cfg S3Config) (*S3, error) {
	endpoint, secure, err := parseMinIOEndpoint(cfg.Endpoint, cfg.Secure)
	if err != nil {
		return nil, err
	}
	cfg.Bucket = strings.TrimSpace(cfg.Bucket)
	if cfg.Bucket == "" {
		return nil, errors.New("MinIO 制品存储桶不能为空")
	}
	if err = s3utils.CheckValidBucketNameStrict(cfg.Bucket); err != nil {
		return nil, fmt.Errorf("MinIO 制品存储桶名称非法: %w", err)
	}
	cfg.AccessKey = strings.TrimSpace(cfg.AccessKey)
	cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)
	if (cfg.AccessKey == "") != (cfg.SecretKey == "") {
		return nil, errors.New("MinIO access_key 和 secret_key 必须同时配置")
	}
	region := lo.CoalesceOrEmpty(strings.TrimSpace(cfg.Region), "us-east-1")

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   50,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:  10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken),
		Secure:       secure,
		Region:       region,
		BucketLookup: minio.BucketLookupPath,
		Transport:    transport,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 MinIO 客户端失败: %w", err)
	}
	prefix := strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	if prefix != "" {
		if _, err = resolveS3Key("", prefix); err != nil {
			return nil, fmt.Errorf("MinIO 制品对象前缀非法: %w", err)
		}
	}
	return &S3{
		client: client,
		bucket: cfg.Bucket,
		prefix: prefix,
	}, nil
}

func (s *S3) Put(ctx context.Context, key string, src io.Reader, options PutOptions) error {
	resolved, err := s.resolve(key)
	if err != nil {
		return err
	}
	hash := sha256.New()
	contentType := lo.CoalesceOrEmpty(strings.TrimSpace(options.ContentType), "application/octet-stream")

	// 第一阶段：写入临时对象。使用纳秒时间戳保证并发写入同 key 时互不冲突。
	partKey := fmt.Sprintf("%s.part.%d", resolved, time.Now().UnixNano())
	defer func() {
		cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = s.client.RemoveObject(cleanCtx, s.bucket, partKey, minio.RemoveObjectOptions{})
	}()

	written, err := s.client.PutObject(ctx, s.bucket, partKey, io.TeeReader(src, hash), options.Size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("上传 MinIO 制品临时对象 %s 失败: %w", partKey, err)
	}

	// 校验阶段：比对大小与 Checksum。校验失败直接退出，正式目标键保持纯净无污染。
	if options.Size >= 0 && written.Size != options.Size {
		return fmt.Errorf("Blob 大小不一致: 预期=%d 实际=%d", options.Size, written.Size)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if options.Checksum != "" && !strings.EqualFold(actual, options.Checksum) {
		return fmt.Errorf("Blob 校验和不一致: 预期=%s 实际=%s", options.Checksum, actual)
	}

	// 第二阶段：服务端原子生效。通过 CopyObject 覆盖到目标对象键，读者只能看到经过完整性校验的成品。
	srcOpts := minio.CopySrcOptions{
		Bucket: s.bucket,
		Object: partKey,
	}
	dstOpts := minio.CopyDestOptions{
		Bucket:      s.bucket,
		Object:      resolved,
		ContentType: contentType,
	}
	if _, err = s.client.CopyObject(ctx, dstOpts, srcOpts); err != nil {
		return fmt.Errorf("提交 MinIO 制品对象 %s 失败: %w", resolved, err)
	}
	return nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	resolved, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, resolved, minio.GetObjectOptions{})
	if err != nil {
		if isMinIONotFound(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return nil, fmt.Errorf("打开 MinIO 制品对象失败: %w", err)
	}
	if _, err = obj.Stat(); err != nil {
		_ = obj.Close()
		if isMinIONotFound(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return nil, fmt.Errorf("读取 MinIO 制品对象元数据失败: %w", err)
	}
	return obj, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	resolved, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err = s.client.RemoveObject(ctx, s.bucket, resolved, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("删除 MinIO 制品对象失败: %w", err)
	}
	return nil
}

func (s *S3) Exists(ctx context.Context, key string) (bool, error) {
	resolved, err := s.resolve(key)
	if err != nil {
		return false, err
	}
	_, err = s.client.StatObject(ctx, s.bucket, resolved, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	if isMinIONotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("查询 MinIO 制品对象元数据失败: %w", err)
}

func (s *S3) resolve(key string) (string, error) {
	return resolveS3Key(s.prefix, key)
}

func resolveS3Key(prefix, key string) (string, error) {
	cleaned, err := cleanAndValidateKey(key)
	if err != nil {
		return "", err
	}
	return lo.Ternary(prefix == "", cleaned, prefix+"/"+cleaned), nil
}

func parseMinIOEndpoint(value string, defaultSecure bool) (string, bool, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", false, errors.New("MinIO 服务地址不能为空")
	}

	// 1. 协议归一化：若未指定协议头，基于 defaultSecure 补齐协议以便收敛到标准 url.Parse 进行完整校验
	hasScheme := strings.Contains(raw, "://")
	targetURL := lo.Ternary(hasScheme, raw, lo.Ternary(defaultSecure, "https://", "http://")+raw)

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return "", false, fmt.Errorf("MinIO 服务地址格式非法: %s", raw)
	}

	scheme := strings.ToLower(parsed.Scheme)
	// 2. 统一合法性规则：仅允许 http/https、必须包含 host、禁止携带 userinfo、path、query 及 fragment
	isInvalid := parsed.Host == "" ||
		parsed.User != nil ||
		!slices.Contains([]string{"http", "https"}, scheme) ||
		strings.Trim(parsed.Path, "/") != "" ||
		parsed.RawQuery != "" ||
		parsed.Fragment != ""
	if isInvalid {
		return "", false, fmt.Errorf("MinIO 服务地址格式非法: %s", raw)
	}

	secure := lo.Ternary(hasScheme, scheme == "https", defaultSecure)
	return parsed.Host, secure, nil
}

func isMinIONotFound(err error) bool {
	var response minio.ErrorResponse
	if !errors.As(err, &response) {
		return false
	}
	return response.StatusCode == 404 || response.Code == "NoSuchKey" ||
		response.Code == "NoSuchObject" || response.Code == "NoSuchBucket" || response.Code == "NotFound"
}
