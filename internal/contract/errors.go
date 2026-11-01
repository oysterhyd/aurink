package contract

import "errors"

// Code 是跨模块错误码。
type Code string

const (
	CodeNotFound    Code = "not_found"
	CodePermission  Code = "permission"
	CodeExists      Code = "exists"
	CodeConflict    Code = "conflict"
	CodeTooLarge    Code = "too_large"
	CodeBinary      Code = "binary"
	CodeEncoding    Code = "encoding"
	CodeIO          Code = "io"
	CodeInvalid     Code = "invalid"
	CodeReadOnly    Code = "read_only"
	CodeCanceled    Code = "canceled"
	CodeUnavailable Code = "unavailable"
	CodeTimeout     Code = "timeout"
	CodeRender      Code = "render"
	CodeInternal    Code = "internal"
)

// Error 是跨模块边界的错误。导出的服务方法返回的非 nil 错误必须是 *Error。
type Error struct {
	Code   Code
	Op     string // 操作，如 "fsstore.Write"
	Path   string // 相关路径（可空）
	Detail string // 面向开发者的细节
	Err    error  // 底层错误
}

func (e *Error) Error() string {
	s := string(e.Code) + ": " + e.Op
	if e.Path != "" {
		s += " " + e.Path
	}
	if e.Detail != "" {
		s += ": " + e.Detail
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// E 构造错误。
func E(code Code, op string, err error) *Error { return &Error{Code: code, Op: op, Err: err} }

// CodeOf 返回错误码：nil 为 ""，非 *Error 为 CodeInternal。
func CodeOf(err error) Code {
	if err == nil {
		return ""
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternal
}
