//go:build windows

package main

import "unsafe"

func fillRect(dc uintptr, r RECT, brush uintptr) {
	pFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), brush)
}
