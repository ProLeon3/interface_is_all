// Package storage 提供中立 JSON 文档读写，不了解待办字段或命令行行为。
package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ReadJSON 返回完整文档；文件不存在时保留 os.ErrNotExist 供业务层初始化状态。
func ReadJSON(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取数据文件：%w", err)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("数据文件 JSON 已损坏，原文件未修改")
	}
	return data, nil
}

// WriteJSON 用同目录临时文件原子替换，只接受有效 JSON，拒绝覆盖已损坏文档。
// 本示例不支持多个进程同时写同一文件。
func WriteJSON(path string, data []byte) error {
	if !json.Valid(data) {
		return fmt.Errorf("不能保存无效 JSON")
	}
	if _, err := ReadJSON(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".todo-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
