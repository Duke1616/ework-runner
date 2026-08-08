package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/s3utils"
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
	if strings.TrimSpace(cfg.Region) == "" {
		cfg.Region = "us-east-1"
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, cfg.SessionToken),
		Secure:       secure,
		Region:       cfg.Region,
		BucketLookup: minio.BucketLookupPath,
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
	contentType := strings.TrimSpace(options.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = s.client.PutObject(ctx, s.bucket, resolved, io.TeeReader(src, hash), options.Size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("上传 MinIO 制品对象 %s 失败: %w", resolved, err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if options.Checksum != "" && !strings.EqualFold(actual, options.Checksum) {
		mismatchErr := fmt.Errorf("Blob 校验和不一致: 预期=%s 实际=%s", options.Checksum, actual)
		if removeErr := s.client.RemoveObject(ctx, s.bucket, resolved, minio.RemoveObjectOptions{}); removeErr != nil {
			return fmt.Errorf("%v，清理 MinIO 对象失败: %w", mismatchErr, removeErr)
		}
		return mismatchErr
	}
	return nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	resolved, err := s.resolve(key)
	if err != nil {
		return err
	}
	if err = s.client.RemoveObject(ctx, s.bucket, resolved, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("删除 MinIO Blob 对象 %s 失败: %w", resolved, err)
	}
	return nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	resolved, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	object, err := s.client.GetObject(ctx, s.bucket, resolved, minio.GetObjectOptions{})
	if err != nil {
		if isMinIONotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("打开 MinIO 制品对象 %s 失败: %w", resolved, err)
	}
	_, err = object.Stat()
	if err != nil {
		_ = object.Close()
		if isMinIONotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("查询 MinIO 制品对象 %s 失败: %w", resolved, err)
	}
	return object, nil
}

func (s *S3) resolve(key string) (string, error) {
	return resolveS3Key(s.prefix, key)
}

func resolveS3Key(prefix, key string) (string, error) {
	key = strings.Trim(strings.TrimSpace(key), "/")
	if key == "" || key == "." || key == ".." || strings.Contains(key, "\\") {
		return "", fmt.Errorf("非法的制品对象键: %q", key)
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("非法的制品对象键: %q", key)
		}
	}
	if prefix == "" {
		return key, nil
	}
	return prefix + "/" + key, nil
}

func parseMinIOEndpoint(value string, defaultSecure bool) (string, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false, errors.New("MinIO 服务地址不能为空")
	}
	if !strings.Contains(value, "://") {
		value = strings.TrimSuffix(value, "/")
		if strings.ContainsAny(value, "/?#") {
			return "", false, fmt.Errorf("MinIO 服务地址格式非法: %s", value)
		}
		return value, defaultSecure, nil
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", false, fmt.Errorf("MinIO 服务地址格式非法: %s", value)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if parsed.Host == "" || parsed.User != nil ||
		(scheme != "http" && scheme != "https") ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false, fmt.Errorf("MinIO 服务地址格式非法: %s", value)
	}
	return parsed.Host, scheme == "https", nil
}

func isMinIONotFound(err error) bool {
	var response minio.ErrorResponse
	if !errors.As(err, &response) {
		return false
	}
	return response.StatusCode == 404 || response.Code == "NoSuchKey" ||
		response.Code == "NoSuchObject" || response.Code == "NoSuchBucket" || response.Code == "NotFound"
}
