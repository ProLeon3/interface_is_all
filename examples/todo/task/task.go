// Package task 维护待办规则，通过 storage 读写中立文档。
package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"example.test/workbench-todo/storage"
)

type Task struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
	// Due 是可选截止日期，格式 YYYY-MM-DD；为空表示没有截止日期，旧文档无需迁移。
	Due string `json:"due,omitempty"`
}

// dueLayout 是截止日期唯一接受的格式。
const dueLayout = "2006-01-02"

// validDue 只接受规范写法的真实日期，空串表示未设置。
func validDue(due string) bool {
	if due == "" {
		return true
	}
	parsed, err := time.Parse(dueLayout, due)
	return err == nil && parsed.Format(dueLayout) == due
}

// next_id 独立保留已使用编号，完成任务不会造成 ID 重用。
type document struct {
	Version int    `json:"version"`
	NextID  uint64 `json:"next_id"`
	Tasks   []Task `json:"tasks"`
}

func read(path string) (document, error) {
	data, err := storage.ReadJSON(path)
	if errors.Is(err, os.ErrNotExist) {
		return document{Version: 1, NextID: 1, Tasks: []Task{}}, nil
	}
	if err != nil {
		return document{}, err
	}
	var doc document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return document{}, fmt.Errorf("待办文档结构无效：%w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return document{}, fmt.Errorf("待办文档包含多余内容")
	}
	if doc.Version != 1 || doc.NextID == 0 || doc.Tasks == nil {
		return document{}, fmt.Errorf("待办文档版本、编号或任务列表无效")
	}
	seen := map[uint64]bool{}
	for _, item := range doc.Tasks {
		if item.ID == 0 || item.ID >= doc.NextID || seen[item.ID] || strings.TrimSpace(item.Title) == "" || !validDue(item.Due) {
			return document{}, fmt.Errorf("待办文档包含重复编号或无效任务")
		}
		seen[item.ID] = true
	}
	sort.Slice(doc.Tasks, func(i, j int) bool { return doc.Tasks[i].ID < doc.Tasks[j].ID })
	return doc, nil
}

func save(path string, doc document) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return storage.WriteJSON(path, append(data, '\n'))
}

// Add 在写入前完成所有业务校验，空标题、无效截止日期或坏文档不会触发持久化。
// due 为空表示不设截止日期。
func Add(path, title, due string) (Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Task{}, fmt.Errorf("标题不能为空")
	}
	due = strings.TrimSpace(due)
	if !validDue(due) {
		return Task{}, fmt.Errorf("截止日期 %q 无效，应为 YYYY-MM-DD", due)
	}
	doc, err := read(path)
	if err != nil {
		return Task{}, err
	}
	if doc.NextID == math.MaxUint64 {
		return Task{}, fmt.Errorf("任务编号已用尽")
	}
	item := Task{ID: doc.NextID, Title: title, Due: due}
	doc.NextID++
	doc.Tasks = append(doc.Tasks, item)
	if err := save(path, doc); err != nil {
		return Task{}, err
	}
	return item, nil
}

// Complete 的第二个返回值说明本次是否发生变化；重复完成不产生额外写入。
func Complete(path string, id uint64) (Task, bool, error) {
	doc, err := read(path)
	if err != nil {
		return Task{}, false, err
	}
	for i := range doc.Tasks {
		if doc.Tasks[i].ID != id {
			continue
		}
		if doc.Tasks[i].Done {
			return doc.Tasks[i], false, nil
		}
		doc.Tasks[i].Done = true
		if err := save(path, doc); err != nil {
			return Task{}, false, err
		}
		return doc.Tasks[i], true, nil
	}
	return Task{}, false, fmt.Errorf("任务 %d 不存在", id)
}

// Pending 每次读取磁盘，返回尚未完成的任务：按截止日期升序，无截止日期的排在最后，
// 截止日期相同时按 ID 升序，保证顺序稳定。
func Pending(path string) ([]Task, error) {
	doc, err := read(path)
	if err != nil {
		return nil, err
	}
	result := []Task{}
	for _, item := range doc.Tasks {
		if !item.Done {
			result = append(result, item)
		}
	}
	// 规范格式 YYYY-MM-DD 的字符串顺序与日期顺序一致，可直接比较。
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Due != b.Due {
			if a.Due == "" || b.Due == "" {
				return b.Due == ""
			}
			return a.Due < b.Due
		}
		return a.ID < b.ID
	})
	return result, nil
}
