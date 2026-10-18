//go:build darwin

package main

/*
#cgo LDFLAGS: -framework Cocoa
void orchestraMoveAppMenuItems(void);
*/
import "C"

func moveAppMenuItems() { C.orchestraMoveAppMenuItems() }
