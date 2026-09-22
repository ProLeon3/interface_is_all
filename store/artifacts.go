package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"os"

	"interfaceisall/internal/jsonfile"
)

// Layout 与模块职责无关，修改坐标不会创建或确认设计版本。
type Layout struct {
	Nodes map[string]Point `json:"nodes"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func (s *Store) SaveLayout(layout Layout) error {
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return s.saveLayout(layout)
}

// UpdateLayout 只合并这次拖动涉及的节点，不用浏览器的旧副本覆盖其他窗口的布局。
// 与完整布局写入共用锁；同时拖动同一节点时，以最后完成的写入为准。
func (s *Store) UpdateLayout(nodes map[string]Point) (Layout, error) {
	if nodes == nil {
		return Layout{}, fmt.Errorf("布局必须提供 nodes 对象")
	}
	unlock, err := s.lock()
	if err != nil {
		return Layout{}, err
	}
	defer unlock()
	layout, err := s.Layout()
	if err != nil && !os.IsNotExist(err) {
		return Layout{}, err
	}
	if layout.Nodes == nil {
		layout.Nodes = map[string]Point{}
	}
	for id, point := range nodes {
		layout.Nodes[id] = point
	}
	return layout, s.saveLayout(layout)
}

func (s *Store) saveLayout(layout Layout) error {
	if layout.Nodes == nil {
		return fmt.Errorf("布局必须提供 nodes 对象")
	}
	for id, point := range layout.Nodes {
		if id == "" || math.IsNaN(point.X) || math.IsNaN(point.Y) || math.IsInf(point.X, 0) || math.IsInf(point.Y, 0) {
			return fmt.Errorf("布局节点或坐标无效：%s", id)
		}
	}
	name, err := s.file("layout.json")
	if err != nil {
		return err
	}
	return jsonfile.Write(name, layout)
}

func (s *Store) Layout() (Layout, error) {
	name, err := s.file("layout.json")
	if err != nil {
		return Layout{}, err
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return Layout{}, err
	}
	var layout Layout
	err = jsonfile.Decode(data, &layout)
	return layout, err
}

// SaveArtifact 在确认版本使用的同一把锁内复核基准并发布观察结果。
// 调用方不能指定覆盖草稿或确认文件的路径，也不能发布已经过期的本次结果。
func (s *Store) SaveArtifact(kind, expectedRevision string, value any) (string, error) {
	if kind != "check" && kind != "review-request" && kind != "review" {
		return "", fmt.Errorf("未知观察结果类型：%s", kind)
	}
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	current, err := s.Confirmed()
	if err != nil {
		return "", err
	}
	if current.Revision != expectedRevision {
		return "", ErrConflict
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	name, err := s.file("observations/" + kind + "-" + hex.EncodeToString(random[:]) + ".json")
	if err != nil {
		return "", err
	}
	return name, jsonfile.Write(name, value)
}
