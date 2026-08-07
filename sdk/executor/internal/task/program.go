package task

import "fmt"

// ProgramKind 描述 Handler 接收的程序形态。
type ProgramKind string

const (
	ProgramInline  ProgramKind = "INLINE"
	ProgramProject ProgramKind = "PROJECT"
)

// Program 是 Executor 为 Handler 准备好的完整程序输入。
type Program struct {
	Kind    ProgramKind
	Inline  *InlineProgram
	Project *ProjectProgram
}

type InlineProgram struct {
	Code string
}

type ProjectProgram struct {
	Root       string
	EntryPoint string
}

// Validate 校验程序的封闭联合结构，不涉及运行时文件系统状态。
func (p *Program) Validate() error {
	if p == nil {
		return fmt.Errorf("程序输入不能为空")
	}
	switch p.Kind {
	case ProgramInline:
		if p.Inline == nil || p.Project != nil || p.Inline.Code == "" {
			return fmt.Errorf("INLINE 程序结构非法或代码为空")
		}
	case ProgramProject:
		if p.Project == nil || p.Inline != nil {
			return fmt.Errorf("PROJECT 程序结构非法")
		}
	default:
		return fmt.Errorf("不支持的程序类型: %s", p.Kind)
	}
	return nil
}
