package workbench

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"interfaceisall/internal/projectpath"
	"interfaceisall/store"
)

// session 不读取设计文件，初始项目设计损坏时仍能取得令牌并选择其他目录。
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"token": s.token, "project": map[string]string{"name": filepath.Base(s.root), "path": s.root}})
}

func selectedDirectory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("请输入项目目录路径")
	}
	// 支持用户习惯的主目录写法，不进行 shell 展开或执行任何命令。
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}
	root, err := projectpath.Root(path)
	if err != nil {
		return "", fmt.Errorf("无法打开目录，请检查路径是否存在且可访问：%w", err)
	}
	return root, nil
}

func (s *Server) atProject(path string) (*Server, error) {
	root, err := selectedDirectory(path)
	if err != nil {
		return nil, err
	}
	storage, err := store.Open(root)
	if err != nil {
		return nil, err
	}
	// 固定本次操作的目录和 Store；切换标签页项目不会改变已发出的写入或扫描目标。
	return &Server{root: root, store: storage, token: s.token, options: s.options, scan: s.scan}, nil
}

func (s *Server) withProject(handle func(*Server, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		selected := s
		if encoded := r.Header.Get("X-Workbench-Project"); encoded != "" {
			path, err := url.PathUnescape(encoded)
			if err != nil {
				writeError(w, http.StatusBadRequest, "项目路径编码无效", nil)
				return
			}
			selected, err = s.atProject(path)
			if err != nil {
				s.fail(w, err)
				return
			}
		}
		handle(selected, w, r)
	}
}

// 打开项目只读取和校验其现有状态，不创建设计、不确认版本，也不改变其他标签页。
func (s *Server) openProject(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path string `json:"path"`
	}
	if !decode(w, r, &input) {
		return
	}
	selected, err := s.atProject(input.Path)
	if err != nil {
		s.fail(w, err)
		return
	}
	selected.state(w, r)
}

type directoryEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	HasGoMod  bool   `json:"has_go_mod"`
	HasDesign bool   `json:"has_design"`
}

func directoryInfo(path string) directoryEntry {
	goMod, goErr := os.Stat(filepath.Join(path, "go.mod"))
	design, designErr := os.Stat(filepath.Join(path, ".architecture"))
	return directoryEntry{
		Name: filepath.Base(path), Path: path,
		HasGoMod: goErr == nil && goMod.Mode().IsRegular(), HasDesign: designErr == nil && design.IsDir(),
	}
}

// 目录浏览只列出直接子目录和项目标记，不上传文件内容；同样要求本机会话令牌。
func (s *Server) directories(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path       string `json:"path"`
		ShowHidden bool   `json:"show_hidden"`
	}
	if !decode(w, r, &input) {
		return
	}
	root, err := selectedDirectory(input.Path)
	if err != nil {
		s.fail(w, err)
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		s.fail(w, fmt.Errorf("无法浏览此目录：%w", err))
		return
	}
	directories := []directoryEntry{}
	for _, entry := range entries {
		if !input.ShowHidden && strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		isDirectory := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			// 项目根目录本身可以是目录链接，选中后解析为真实路径。
			info, err := os.Stat(path)
			isDirectory = err == nil && info.IsDir()
		}
		if isDirectory {
			directories = append(directories, directoryInfo(path))
		}
	}
	writeJSON(w, map[string]any{"current": directoryInfo(root), "parent": filepath.Dir(root), "directories": directories})
}
