//go:build linux || darwin

package output

import (
	"os"
	"syscall"
	"unsafe"
)

// termiosStruct 用于 TCGETS/TCGETA ioctl 判断文件描述符是否为终端。
// 字段内容本身不被使用，仅用于提供正确的缓冲区大小与对齐。
type termiosStruct struct {
	Iflag  uint32
	Oflag  uint32
	Cflag  uint32
	Lflag  uint32
	Line   byte
	Cc     [19]byte
	Ispeed uint32
	Ospeed uint32
}

// unixIoctl 对 fd 执行 TCGETS ioctl。
// 成功（fd 为终端）返回 nil；失败（非终端）返回 errno。
//
// 实现说明：
//   - Linux 使用 TCGETS (0x5401)
//   - macOS/BSD 使用 TIOCGETA (0x40487413)
//   - 在大多数 Unix 上，对非终端 fd 调用会返回 ENOTTY
func unixIoctl(fd int, t *termiosStruct) (uintptr, error) {
	req := uintptr(syscall.TCGETS) // Linux
	if req == 0 {
		// 非 Linux（macOS 等）回退到 TIOCGETA
		req = 0x40487413
	}
	r1, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		req,
		uintptr(unsafe.Pointer(t)),
	)
	if err != 0 {
		return r1, err
	}
	return r1, nil
}

// isTerminal 判断文件是否为终端。
// 通过 ioctl(fd, TCGETS/TCGETA, ...) 判断，成功即为终端；
// 对非终端（管道/文件）返回 ENOTTY。
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fd := int(f.Fd())
	var termios termiosStruct
	_, err := unixIoctl(fd, &termios)
	return err == nil
}
