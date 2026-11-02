package workspace

import (
	"errors"
	"os"

	"aurink/internal/contract"
)

func failure(code contract.Code, op, path string, err error) *contract.Error {
	return &contract.Error{Code: code, Op: op, Path: path, Err: err}
}

func ioFailure(op, path string, err error) *contract.Error {
	code := contract.CodeIO
	switch {
	case errors.Is(err, os.ErrNotExist):
		code = contract.CodeNotFound
	case errors.Is(err, os.ErrExist):
		code = contract.CodeExists
	case errors.Is(err, os.ErrPermission):
		code = contract.CodePermission
	}
	return failure(code, op, path, err)
}
