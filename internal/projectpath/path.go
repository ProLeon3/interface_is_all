// Package projectpath 检查项目内路径，避免符号链接改变目录归属或写入位置。
package projectpath

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Root 将用户选定的项目根目录解析为真实的绝对目录。
func Root(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("项目根路径不是目录：%s", root)
	}
	return abs, nil
}

// Resolve 允许尚不存在的路径，但拒绝项目内部的符号链接和路径越界。
func Resolve(root, relative string) (string, error) {
	relative = filepath.FromSlash(relative)
	if filepath.IsAbs(relative) || relative != filepath.Clean(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径必须位于项目中：%s", relative)
	}
	current := root
	for _, segment := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return filepath.Join(root, relative), nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("项目内部路径不能经过符号链接：%s", current)
		}
	}
	return current, nil
}

// Relative 对外部依赖返回 false，不按 import 字符串猜测其归属。
func Relative(root, absolute string) (string, bool) {
	relative, err := filepath.Rel(root, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(relative), true
}
