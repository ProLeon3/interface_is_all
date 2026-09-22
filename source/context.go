package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"interfaceisall/design"
)

// 首版提供完整文件，不截断函数或抽样后宣称覆盖完整项目。
const MaxContextBytes = 1 << 20

var ErrStale = errors.New("源码或构建范围已变化，请放弃旧分析后重新分析代码")

// Context 将源码、依赖清单与构建范围绑定为一次不可变的分析输入。
type Context struct {
	Fingerprint string `json:"fingerprint"`
	Facts       Facts  `json:"facts"`
}

func EmptyDesign() design.Design {
	return design.Design{SchemaVersion: 1, Modules: []design.Module{}, Interfaces: []design.Interface{}, ForbiddenDependencies: []design.ForbiddenDependency{}}
}

func Fingerprint(f Facts) string {
	data, _ := json.Marshal(f)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Collect 不依赖任何草稿；诊断随上下文返回，供页面展示实际读取范围。
func Collect(ctx context.Context, root string, options Options) (Context, error) {
	facts, err := Scan(ctx, root, EmptyDesign(), options)
	if err != nil {
		return Context{}, err
	}
	return Context{Fingerprint: Fingerprint(facts), Facts: facts}, nil
}

func (c Context) Validate() error {
	if c.Fingerprint == "" || c.Fingerprint != Fingerprint(c.Facts) {
		return fmt.Errorf("源码上下文指纹无效")
	}
	if len(c.Facts.Diagnostics) > 0 {
		return fmt.Errorf("源码加载不完整，请先处理范围与诊断中的问题")
	}
	if len(c.Facts.Files) == 0 {
		return fmt.Errorf("当前构建范围内没有可分析的 Go 源码")
	}
	data, _ := json.Marshal(c)
	if len(data) > MaxContextBytes {
		return fmt.Errorf("完整源码上下文为 %d 字节，超过首版 1 MiB 上限；未截断或提交给模型", len(data))
	}
	return nil
}

// Verify 在生成完成、接受及确认本次分析时重采集，拒绝把旧源码结论写成当前基准。
func (c Context) Verify(ctx context.Context, root string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	latest, err := Collect(ctx, root, Options{Tags: c.Facts.Scope.Tags})
	if err != nil {
		return fmt.Errorf("无法复核分析源码：%w", err)
	}
	if latest.Fingerprint != c.Fingerprint {
		return ErrStale
	}
	return ctx.Err()
}
