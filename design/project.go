package design

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"interfaceisall/internal/projectpath"
)

// ValidateProject 在纯设计校验之外检查已有目录，未实现的模块仍可保存。
func ValidateProject(root string, d Design) error {
	if err := Validate(d); err != nil {
		return err
	}
	root, err := projectpath.Root(root)
	if err != nil {
		return err
	}
	issues := []Issue{}
	for i, m := range d.Modules {
		absolute, err := projectpath.Resolve(root, m.Root)
		if err == nil {
			var info os.FileInfo
			info, err = os.Stat(absolute)
			if os.IsNotExist(err) {
				err = nil
			} else if err == nil && !info.IsDir() {
				err = fmt.Errorf("模块根路径不是目录：%s", m.Root)
			}
		}
		if err != nil {
			issues = append(issues, Issue{"module_directory", fmt.Sprintf("/modules/%d/root", i), err.Error()})
		}
	}
	if len(issues) > 0 {
		return &ValidationError{issues}
	}
	return nil
}

// Fingerprint 忽略 JSON 缩进和对象键顺序，用于绑定用户实际审阅的内容。
func Fingerprint(d Design) string {
	data, _ := json.Marshal(d)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
