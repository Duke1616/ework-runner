// Package variable 定义跨领域传递的变量数据结构。
package variable

// Item 表示运行时传递的变量项。
// 它只描述变量业务值，不包含作用域、租户和数据库主键等持久化信息。
type Item struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}
