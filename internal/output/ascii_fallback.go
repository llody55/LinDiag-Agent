package output

// ============================================================
// ASCII 降级映射（方案 B）
//
// 当终端不支持 Unicode（裸 Linux 控制台、无 UTF-8 locale、
// 非终端重定向）时，asciiMode=true，所有 Unicode 图标和制表符
// 会被替换为 ASCII 等价物，避免输出问号或空白。
//
// 中文文字本身保留——只要 UTF-8 字节正确，多数现代 SSH 终端
// 可读；真正无法显示的终端至少 ASCII 边框不错位、图标不显示 ?。
// ============================================================

// icon 返回图标字符：Unicode 模式原样返回，ASCII 模式返回替代标记。
func icon(unicode, ascii string) string {
	if asciiMode {
		return ascii
	}
	return unicode
}

// 制表符的 Unicode 原字符与 ASCII 替代。
// boxTR/boxBR 本项目暂未使用，但保留以便 HLine 的 isBoxChar 判断完整。
const (
	boxTL = "┌" // 左上角
	boxTR = "┐" // 右上角（本项目未使用，保留对称）
	boxBL = "└" // 左下角
	boxBR = "┘" // 右下角（本项目未使用，保留对称）
	boxML = "├" // 左中（分隔线）
	boxH  = "─" // 横线
	boxV  = "│" // 竖线

	dboxTL = "╔" // 双线左上
	dboxBL = "╚" // 双线左下
	dboxH  = "═" // 双线横线
	dboxV  = "║" // 双线竖线
)

// ASCII 替代制表符。
const (
	asciiBoxTL = "+"
	asciiBoxTR = "+"
	asciiBoxBL = "+"
	asciiBoxBR = "+"
	asciiBoxML = "+"
	asciiBoxH  = "-"
	asciiBoxV  = "|"

	asciiDBoxTL = "+"
	asciiDBoxBL = "+"
	asciiDBoxH  = "="
	asciiDBoxV  = "|"
)

// boxTLChar 返回单线左上角字符。
func boxTLChar() string {
	if asciiMode {
		return asciiBoxTL
	}
	return boxTL
}

// boxBLChar 返回单线左下角字符。
func boxBLChar() string {
	if asciiMode {
		return asciiBoxBL
	}
	return boxBL
}

// boxMLChar 返回单线左中（分隔线）字符。
func boxMLChar() string {
	if asciiMode {
		return asciiBoxML
	}
	return boxML
}

// boxVChar 返回单线竖线字符。
func boxVChar() string {
	if asciiMode {
		return asciiBoxV
	}
	return boxV
}

// dBoxTLChar 返回双线左上角字符。
func dBoxTLChar() string {
	if asciiMode {
		return asciiDBoxTL
	}
	return dboxTL
}

// dBoxBLChar 返回双线左下角字符。
func dBoxBLChar() string {
	if asciiMode {
		return asciiDBoxBL
	}
	return dboxBL
}

// dBoxVChar 返回双线竖线字符。
func dBoxVChar() string {
	if asciiMode {
		return asciiDBoxV
	}
	return dboxV
}
