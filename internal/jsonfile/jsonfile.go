// Package jsonfile 统一 JSON 边界和原子文件替换，避免半写入与歧义输入。
package jsonfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Decode 拒绝重复键、未知字段和尾随 JSON，防止不同读取端解释出不同内容。
func Decode(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := checkValue(d); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("JSON 后存在多余内容")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	d.UseNumber()
	return d.Decode(value)
}

func checkValue(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	keys := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("JSON 对象键必须是字符串")
			}
			if keys[name] {
				return fmt.Errorf("JSON 重复键 %q", name)
			}
			keys[name] = true
		}
		if err := checkValue(d); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}

// Write 在目标目录创建临时文件，完整写入并同步后再替换目标。
func Write(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(name), ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), name); err != nil {
		return err
	}
	// 同步目录项，使确认快照的发布顺序在进程或系统异常后仍有持久化保障。
	dir, err := os.Open(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
