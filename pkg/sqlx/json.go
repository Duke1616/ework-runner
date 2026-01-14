package sqlx

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONColumn 代表存储字段的 json 类型
// 主要用于没有提供默认 json 类型的数据库
// T 可以是结构体，也可以是切片或者 map
// 理论上来说一切可以被 json 库所处理的类型都能被用作 T
// 不建议使用指针作为 T 的类型
// 如果 T 是指针，那么在 Val 为 nil 的情况下，一定要把 Valid 设置为 false
type JSONColumn[T any] struct {
	Val   T
	Valid bool
}

// Value 返回一个 json 串。类型是 string
//
//nolint:nilnil //忽略
func (j JSONColumn[T]) Value() (driver.Value, error) {
	if !j.Valid {
		return nil, nil
	}
	res, err := json.Marshal(j.Val)
	return string(res), err
}

// Scan 将 src 转化为对象
// src 的类型必须是 []byte, string 或者 nil
// 如果是 nil，我们不会做任何处理
func (j *JSONColumn[T]) Scan(src any) error {
	var bs []byte
	switch val := src.(type) {
	case nil:
		return nil
	case []byte:
		bs = val
	case string:
		bs = []byte(val)
	default:
		return fmt.Errorf("ekit：JSONColumn.Scan 不支持 src 类型 %v", src)
	}

	if err := json.Unmarshal(bs, &j.Val); err != nil {
		return err
	}
	j.Valid = true
	return nil
}
