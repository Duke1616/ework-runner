package blobstore

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
)

// cleanAndValidateKey 校验并规范化制品对象键，防止目录穿越与非法字符。
func cleanAndValidateKey(key string) (string, error) {
	raw := key
	key = strings.Trim(strings.TrimSpace(key), "/")
	if key == "" || strings.Contains(key, "\\") || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, raw)
	}
	segments := lo.Map(strings.Split(key, "/"), func(s string, _ int) string {
		return strings.TrimSpace(s)
	})
	if lo.SomeBy(segments, func(s string) bool { return s == "" || s == "." || s == ".." }) {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, raw)
	}
	return strings.Join(segments, "/"), nil
}
