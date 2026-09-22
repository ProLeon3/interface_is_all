package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Observation 保留报告文件名和保存时间，工作台只将报告作为某次扫描的观察结果。
type Observation struct {
	Name      string          `json:"name"`
	UpdatedAt time.Time       `json:"updated_at"`
	Report    json.RawMessage `json:"report"`
}

// LatestObservation 与命令行共享观察目录，不根据随机文件名推断先后顺序。
// 缺少报告返回 nil；最新报告损坏时报告错误，不能静默退回旧的成功结果。
func (s *Store) LatestObservation(kind string) (*Observation, error) {
	if kind != "check" && kind != "review" {
		return nil, fmt.Errorf("不支持的报告类型：%s", kind)
	}
	dir, err := s.file("observations")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var candidates []os.FileInfo
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, kind+"-") || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, "review-request-") {
			continue
		}
		// 拒绝符号链接，保持与其他存储接口相同的项目边界。
		if _, err := s.file("observations/" + name); err != nil {
			return nil, err
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("观察结果不是普通文件：%s", name)
		}
		candidates = append(candidates, info)
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ModTime().Equal(candidates[j].ModTime()) {
			return candidates[i].Name() > candidates[j].Name()
		}
		return candidates[i].ModTime().After(candidates[j].ModTime())
	})
	latest := candidates[0]
	name, err := s.file("observations/" + latest.Name())
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("观察结果 JSON 已损坏：%s", latest.Name())
	}
	return &Observation{latest.Name(), latest.ModTime().UTC(), data}, nil
}
