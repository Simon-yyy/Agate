//go:build windows

package agent

import (
	"fmt"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	modVersion                  = syscall.NewLazyDLL("version.dll")
	procGetFileVersionInfoSizeW = modVersion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = modVersion.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = modVersion.NewProc("VerQueryValueW")
)

func defaultReadExecutableProductVersion(path string) string {
	if path == "" {
		return ""
	}
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	size, _, _ := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(name)), 0)
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	ok, _, _ := procGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(name)), 0, size, uintptr(unsafe.Pointer(&buf[0])))
	if ok == 0 {
		return ""
	}
	lang, cp, found := firstFileVersionTranslation(buf)
	if !found {
		lang, cp = 0x0409, 0x04b0
	}
	product := queryFileVersionString(buf, fmt.Sprintf(`\StringFileInfo\%04x%04x\ProductVersion`, lang, cp))
	if product == "" {
		product = queryFileVersionString(buf, fmt.Sprintf(`\StringFileInfo\%04x%04x\FileVersion`, lang, cp))
	}
	return product
}

func firstFileVersionTranslation(block []byte) (uint16, uint16, bool) {
	sub, err := syscall.UTF16PtrFromString(`\VarFileInfo\Translation`)
	if err != nil {
		return 0, 0, false
	}
	var ptr unsafe.Pointer
	var length uint32
	ok, _, _ := procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&block[0])),
		uintptr(unsafe.Pointer(sub)),
		uintptr(unsafe.Pointer(&ptr)),
		uintptr(unsafe.Pointer(&length)),
	)
	if ok == 0 || ptr == nil || length < 4 {
		return 0, 0, false
	}
	raw := unsafe.Slice((*byte)(ptr), 4)
	lang := uint16(raw[0]) | uint16(raw[1])<<8
	cp := uint16(raw[2]) | uint16(raw[3])<<8
	return lang, cp, true
}

func queryFileVersionString(block []byte, key string) string {
	sub, err := syscall.UTF16PtrFromString(key)
	if err != nil {
		return ""
	}
	var ptr unsafe.Pointer
	var length uint32
	ok, _, _ := procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&block[0])),
		uintptr(unsafe.Pointer(sub)),
		uintptr(unsafe.Pointer(&ptr)),
		uintptr(unsafe.Pointer(&length)),
	)
	if ok == 0 || ptr == nil || length == 0 {
		return ""
	}
	n := int(length)
	if n > 0 {
		n--
	}
	chars := unsafe.Slice((*uint16)(ptr), n)
	return strings.TrimSpace(strings.ReplaceAll(string(utf16.Decode(chars)), "\x00", ""))
}
