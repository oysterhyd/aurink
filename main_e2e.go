//go:build e2e

package main

import (
	"os"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/plugins/automation"
)

// An end-to-end build (tests/e2e) is driven over MYGO_AUTOMATION and keeps its
// data in AURINK_USER_DATA, away from the user's.
func init() {
	if dir := os.Getenv("AURINK_USER_DATA"); dir != "" {
		mygo.App.SetPath(mygo.PathUserData, dir)
	}
}
