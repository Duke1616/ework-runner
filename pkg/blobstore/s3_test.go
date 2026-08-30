package blobstore

import (
	"errors"
	"net/http"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

func TestParseMinIOEndpoint(t *testing.T) {
	testCases := []struct {
		name          string
		value         string
		defaultSecure bool
		endpoint      string
		secure        bool
	}{
		{name: "http协议端点", value: "http://minio:9000", endpoint: "minio:9000"},
		{name: "https协议端点", value: "https://s3.example.com/", endpoint: "s3.example.com", secure: true},
		{name: "无协议头端点", value: "minio:9000", endpoint: "minio:9000"},
		{name: "带末尾斜杠无协议头端点", value: "minio:9000/", endpoint: "minio:9000"},
		{name: "默认开启https安全连接", value: "s3.example.com", defaultSecure: true, endpoint: "s3.example.com", secure: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, secure, err := parseMinIOEndpoint(tc.value, tc.defaultSecure)
			require.NoError(t, err)
			require.Equal(t, tc.endpoint, endpoint)
			require.Equal(t, tc.secure, secure)
		})
	}
}

func TestParseMinIOEndpoint_RejectsInvalidValue(t *testing.T) {
	testCases := []struct {
		name   string
		value  string
		errMsg string
	}{
		{name: "空端点地址", value: "", errMsg: "不能为空"},
		{name: "不支持的协议头", value: "ftp://minio:9000", errMsg: "格式非法"},
		{name: "带多余path路径", value: "http://minio:9000/path", errMsg: "格式非法"},
		{name: "带多余path且无协议", value: "minio:9000/path", errMsg: "格式非法"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseMinIOEndpoint(tc.value, false)
			require.Error(t, err)
			require.ErrorContains(t, err, tc.errMsg)
		})
	}
}

func TestNewS3_ConfigValidation(t *testing.T) {
	testCases := []struct {
		name      string
		cfg       S3Config
		wantErr   string
		checkFunc func(t *testing.T, s *S3)
	}{
		{
			name: "成功_完整合法配置",
			cfg: S3Config{
				Endpoint:  "http://minio:9000",
				Bucket:    "etask-artifacts",
				Prefix:    "codebook/releases",
				AccessKey: "access-key",
				SecretKey: "secret-key",
			},
			checkFunc: func(t *testing.T, s *S3) {
				require.Equal(t, "etask-artifacts", s.bucket)
				require.Equal(t, "codebook/releases", s.prefix)
			},
		},
		{
			name: "成功_使用默认Region",
			cfg: S3Config{
				Endpoint:  "http://minio:9000",
				Bucket:    "etask-artifacts",
				AccessKey: "access-key",
				SecretKey: "secret-key",
			},
			checkFunc: func(t *testing.T, s *S3) {
				require.Equal(t, "etask-artifacts", s.bucket)
				require.Equal(t, "", s.prefix)
			},
		},
		{
			name:    "失败_存储桶名称非法",
			cfg:     S3Config{Endpoint: "minio:9000", Bucket: "INVALID_BUCKET"},
			wantErr: "存储桶名称非法",
		},
		{
			name:    "失败_前缀包含目录穿越",
			cfg:     S3Config{Endpoint: "minio:9000", Bucket: "etask-artifacts", Prefix: "../outside"},
			wantErr: "对象前缀非法",
		},
		{
			name:    "失败_仅配置AccessKey缺少SecretKey",
			cfg:     S3Config{Endpoint: "minio:9000", Bucket: "etask-artifacts", AccessKey: "access-key"},
			wantErr: "必须同时配置",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewS3(tc.cfg)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.checkFunc != nil {
				tc.checkFunc(t, store)
			}
		})
	}
}

func TestS3_KeyResolution(t *testing.T) {
	testCases := []struct {
		name      string
		prefix    string
		key       string
		wantKey   string
		errTarget error
	}{
		{
			name:    "带前缀正常解析",
			prefix:  "codebook",
			key:     "artifacts/release.tar.zst",
			wantKey: "codebook/artifacts/release.tar.zst",
		},
		{
			name:    "无前缀正常解析",
			prefix:  "",
			key:     "artifacts/release.tar.zst",
			wantKey: "artifacts/release.tar.zst",
		},
		{
			name:      "拒绝父级目录穿越",
			prefix:    "codebook",
			key:       "../outside",
			errTarget: ErrInvalidKey,
		},
		{
			name:      "拒绝反斜杠字符",
			prefix:    "codebook",
			key:       "dir\\sub",
			errTarget: ErrInvalidKey,
		},
		{
			name:      "拒绝绝对路径",
			prefix:    "codebook",
			key:       "/root/object",
			errTarget: ErrInvalidKey,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := &S3{prefix: tc.prefix}
			resolved, err := store.resolve(tc.key)
			if tc.errTarget != nil {
				require.ErrorIs(t, err, tc.errTarget)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantKey, resolved)
		})
	}
}

func TestIsMinIONotFound(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "NoSuchKey错误码",
			err:  minio.ErrorResponse{Code: "NoSuchKey", StatusCode: http.StatusNotFound},
			want: true,
		},
		{
			name: "NoSuchObject错误码",
			err:  minio.ErrorResponse{Code: "NoSuchObject", StatusCode: http.StatusNotFound},
			want: true,
		},
		{
			name: "NoSuchBucket错误码",
			err:  minio.ErrorResponse{Code: "NoSuchBucket", StatusCode: http.StatusNotFound},
			want: true,
		},
		{
			name: "NotFound错误码",
			err:  minio.ErrorResponse{Code: "NotFound", StatusCode: http.StatusNotFound},
			want: true,
		},
		{
			name: "404状态码无特定Code",
			err:  minio.ErrorResponse{StatusCode: http.StatusNotFound},
			want: true,
		},
		{
			name: "非404其他错误",
			err:  minio.ErrorResponse{Code: "AccessDenied", StatusCode: http.StatusForbidden},
			want: false,
		},
		{
			name: "标准库普通错误",
			err:  errors.New("connection reset"),
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isMinIONotFound(tc.err))
		})
	}
}
