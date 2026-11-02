// Package trash adapts the MyGo shell's trash operation to the contract.
package trash

import (
	"errors"
	"os"

	"aurink/internal/contract"
	"github.com/egoist/mygo"
)

// Trasher moves files to the system trash after the application is ready.
type Trasher struct{}

var _ contract.Trasher = Trasher{}

// New returns a system trasher.
func New() Trasher { return Trasher{} }

// Trash moves path to the system trash; it never falls back to deletion.
func (Trasher) Trash(path string) error {
	if _, err := os.Lstat(path); err != nil {
		return failure(path, err)
	}
	if err := mygo.Shell.TrashItem(path); err != nil {
		return failure(path, err)
	}
	return nil
}

func failure(path string, err error) *contract.Error {
	code := contract.CodeIO
	switch {
	case errors.Is(err, os.ErrNotExist):
		code = contract.CodeNotFound
	case errors.Is(err, os.ErrPermission):
		code = contract.CodePermission
	}
	return &contract.Error{Code: code, Op: "trash", Path: path, Err: err}
}
