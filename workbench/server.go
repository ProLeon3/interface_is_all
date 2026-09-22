// Package workbench 将已有设计与检查能力提供给本地浏览器，不另建一份设计存储。
package workbench

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"interfaceisall/check"
	"interfaceisall/design"
	"interfaceisall/internal/jsonfile"
	"interfaceisall/internal/projectpath"
	"interfaceisall/source"
	"interfaceisall/store"
)

// 静态资源随 Go 二进制分发，使用工作台无需安装 Node 或访问 CDN。
//
//go:embed web/*
var assets embed.FS

type Options struct {
	Tags    string
	Timeout time.Duration
}

type Server struct {
	root    string
	store   *store.Store
	token   string
	options Options
	scan    chan struct{}
	handler http.Handler
}

func New(root string, options Options) (*Server, error) {
	root, err := projectpath.Root(root)
	if err != nil {
		return nil, err
	}
	s, err := store.Open(root)
	if err != nil {
		return nil, err
	}
	if options.Timeout <= 0 {
		options.Timeout = 2 * time.Minute
	}
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, err
	}
	server := &Server{root: root, store: s, token: hex.EncodeToString(token[:]), options: options, scan: make(chan struct{}, 1)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", server.session)
	mux.HandleFunc("POST /api/projects/open", server.openProject)
	mux.HandleFunc("POST /api/directories", server.directories)
	// 请求显式携带所在标签页选择的项目，服务不维护一个会被其他窗口切换的全局项目。
	mux.HandleFunc("GET /api/state", server.withProject((*Server).state))
	mux.HandleFunc("PUT /api/draft", server.withProject((*Server).saveDraft))
	mux.HandleFunc("POST /api/validate", server.withProject((*Server).validate))
	mux.HandleFunc("POST /api/confirm", server.withProject((*Server).confirm))
	mux.HandleFunc("POST /api/design-request", server.withProject((*Server).designRequest))
	mux.HandleFunc("POST /api/proposal-import", server.withProject((*Server).importProposal))
	mux.HandleFunc("POST /api/proposals/{id}/discard", server.withProject((*Server).discardProposal))
	mux.HandleFunc("POST /api/initial-analysis/discard", server.withProject((*Server).discardInitial))
	mux.HandleFunc("POST /api/reanalysis/discard", server.withProject((*Server).discardInitial))
	mux.HandleFunc("GET /api/proposals/{id}", server.withProject((*Server).proposal))
	mux.HandleFunc("POST /api/proposals/{id}/accept", server.withProject((*Server).acceptProposal))
	mux.HandleFunc("GET /api/handoff", server.withProject((*Server).handoff))
	mux.HandleFunc("PATCH /api/layout", server.withProject((*Server).layout))
	mux.HandleFunc("GET /api/observations", server.withProject((*Server).observations))
	mux.HandleFunc("POST /api/check", server.withProject((*Server).runCheck))
	mux.HandleFunc("POST /api/review-request", server.withProject((*Server).runCheck))
	mux.HandleFunc("POST /api/review-import", server.withProject((*Server).importReview))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "接口不存在", nil)
	})
	static, _ := fs.Sub(assets, "web")
	mux.Handle("/", http.FileServer(http.FS(static)))
	server.handler = mux
	return server, nil
}

// ServeHTTP 限制本地 Host、同源请求和写入令牌，避免网页跨站读取源码或确认设计。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		host = r.Host
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		writeError(w, http.StatusForbidden, "工作台仅接受本机访问", nil)
		return
	}
	if origin := r.Header.Get("Origin"); (origin != "" && origin != "http://"+r.Host) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeError(w, http.StatusForbidden, "工作台仅接受同源请求", nil)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Workbench-Token")), []byte(s.token)) != 1 {
			writeError(w, http.StatusForbidden, "会话已变化，请刷新页面后重试", nil)
			return
		}
		media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if media != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "请求必须使用 application/json", nil)
			return
		}
	}
	s.handler.ServeHTTP(w, r)
}

// Listen 只绑定回环地址；检查超时作用于单次扫描，工作台自身持续运行到收到退出信号。
func Listen(ctx context.Context, root, address string, options Options, out io.Writer) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("监听地址必须包含本机地址和端口：%w", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("工作台必须监听回环地址，例如 127.0.0.1:8090")
	}
	handler, err := New(root, options)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: time.Minute, BaseContext: func(net.Listener) context.Context { return ctx }}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		case <-done:
		}
	}()
	fmt.Fprintf(out, "工作台已启动：http://%s\n关联项目：%s\n", listener.Addr(), handler.root)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	d, hash, err := s.store.Draft()
	if err != nil && !os.IsNotExist(err) {
		s.fail(w, err)
		return
	}
	var draft *design.Design
	if err == nil {
		draft = &d
	}
	current, err := s.store.Confirmed()
	if err != nil && !errors.Is(err, store.ErrUnconfirmed) {
		s.fail(w, err)
		return
	}
	var confirmed *store.Snapshot
	if err == nil {
		confirmed = &current
	}
	layout, err := s.store.Layout()
	if err != nil && !os.IsNotExist(err) {
		s.fail(w, err)
		return
	}
	if layout.Nodes == nil {
		layout.Nodes = map[string]store.Point{}
	}
	initial, err := s.store.InitialAnalysis()
	if err != nil {
		s.fail(w, err)
		return
	}
	pending, err := s.store.PendingDesignProposal()
	if err != nil {
		s.fail(w, err)
		return
	}
	goMod, goErr := os.Stat(filepath.Join(s.root, "go.mod"))
	writeJSON(w, map[string]any{
		"project": map[string]string{"name": filepath.Base(s.root), "path": s.root},
		"draft":   draft, "draft_hash": hash, "confirmed": confirmed, "layout": layout,
		"token": s.token, "tags": s.options.Tags, "pending_proposal_id": pending,
		"initial_analysis": initial, "initial_analysis_available": confirmed == nil && goErr == nil && goMod.Mode().IsRegular(),
		"reanalysis_available": confirmed != nil && goErr == nil && goMod.Mode().IsRegular(),
	})
}

func (s *Server) saveDraft(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Design           json.RawMessage `json:"design"`
		ExpectedHash     *string         `json:"expected_draft_hash"`
		ExpectedRevision *string         `json:"expected_revision"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.ExpectedHash == nil || input.ExpectedRevision == nil {
		writeError(w, 400, "保存草稿必须提供读取时的草稿指纹和基准版本", nil)
		return
	}
	d, err := design.Parse(input.Design)
	if err != nil {
		s.fail(w, err)
		return
	}
	hash, err := s.store.SaveDraftIfUnchanged(d, *input.ExpectedHash, *input.ExpectedRevision)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]string{"draft_hash": hash})
}

// 导入先校验原始 JSON，保留重复键检查；通过后仍须用户主动保存草稿。
func (s *Server) validate(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "设计文件读取失败或超过 32 MiB", nil)
		return
	}
	d, err := design.Parse(data)
	if err == nil {
		err = design.ValidateProject(s.root, d)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, d)
}

func (s *Server) confirm(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DraftHash        string  `json:"draft_hash"`
		ExpectedRevision *string `json:"expected_revision"`
		Actor            string  `json:"actor"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.ExpectedRevision == nil || strings.TrimSpace(input.Actor) == "" || input.DraftHash == "" {
		writeError(w, 400, "请提供已审阅草稿、基准版本和确认人", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.options.Timeout)
	defer cancel()
	snapshot, err := s.store.ConfirmContext(ctx, input.DraftHash, *input.ExpectedRevision, input.Actor)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, snapshot)
}

func (s *Server) layout(w http.ResponseWriter, r *http.Request) {
	var layout store.Layout
	if !decode(w, r, &layout) {
		return
	}
	merged, err := s.store.UpdateLayout(layout.Nodes)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, merged)
}

func (s *Server) observations(w http.ResponseWriter, r *http.Request) {
	result := map[string]any{}
	warnings := []string{}
	for _, kind := range []string{"check", "review"} {
		observation, err := s.store.LatestObservation(kind)
		if err == nil && observation != nil {
			// 读取时再次检查报告形状，不能让损坏的本地文件变成浏览器里的通过结论。
			if kind == "check" {
				var report check.Report
				err = jsonfile.Decode(observation.Report, &report)
				if err == nil && (report.SchemaVersion != 1 || report.DesignRevision == "" || report.SemanticStatus != "not_run" || report.Facts.Packages == nil || report.Facts.Files == nil || report.Deterministic.Violations == nil || (report.Deterministic.Status != "passed" && report.Deterministic.Status != "violations" && report.Deterministic.Status != "incomplete")) {
					err = fmt.Errorf("确定性报告缺少有效状态或设计基准")
				}
				if err == nil {
					observation.Report, err = json.Marshal(report)
				}
			} else {
				var report check.SemanticReport
				err = jsonfile.Decode(observation.Report, &report)
				if err == nil && (report.SchemaVersion != 1 || report.DesignRevision == "" || report.RequestID == "" || report.Interfaces == nil || report.Concerns == nil || report.Status != "pending_confirmation") {
					err = fmt.Errorf("语义报告缺少有效状态或设计基准")
				}
				if err == nil {
					observation.Report, err = json.Marshal(report)
				}
			}
		}
		if err != nil {
			warnings = append(warnings, kind+"："+err.Error())
			observation = nil
		}
		result[kind] = observation
	}
	result["warnings"] = warnings
	writeJSON(w, result)
}

// 扫描与导入共享限流和取消上下文，重复点击不会并行启动多次 Go 扫描。
func (s *Server) startScan(w http.ResponseWriter, r *http.Request) (context.Context, func(), bool) {
	select {
	case s.scan <- struct{}{}:
		ctx, cancel := context.WithTimeout(r.Context(), s.options.Timeout)
		return ctx, func() { cancel(); <-s.scan }, true
	default:
		writeError(w, http.StatusConflict, "已有检查正在运行，请等待结束", nil)
		return nil, nil, false
	}
}

func (s *Server) runCheck(w http.ResponseWriter, r *http.Request) {
	ctx, release, ok := s.startScan(w, r)
	if !ok {
		return
	}
	defer release()
	current, err := s.store.Confirmed()
	if err != nil {
		s.fail(w, err)
		return
	}
	report, err := check.Run(ctx, s.root, current, check.Options{Tags: s.options.Tags})
	if err == nil {
		_, err = s.store.SaveArtifact("check", current.Revision, report)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if r.URL.Path == "/api/review-request" {
		request, err := check.NewReviewRequest(current, report.Facts)
		if err == nil {
			_, err = s.store.SaveArtifact("review-request", current.Revision, request)
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, request)
		return
	}
	writeJSON(w, report)
}

func (s *Server) importReview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Request  json.RawMessage `json:"request"`
		Response json.RawMessage `json:"response"`
	}
	if !decode(w, r, &input) {
		return
	}
	request, err := check.ParseReviewRequest(input.Request)
	if err != nil {
		s.fail(w, err)
		return
	}
	response, err := check.ParseReviewResponse(input.Response)
	if err != nil {
		s.fail(w, err)
		return
	}
	ctx, release, ok := s.startScan(w, r)
	if !ok {
		return
	}
	defer release()
	current, err := s.store.Confirmed()
	if err != nil {
		s.fail(w, err)
		return
	}
	report, err := check.ImportReview(ctx, s.root, current, request, response)
	if err == nil {
		_, err = s.store.SaveArtifact("review", current.Revision, report)
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, report)
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	// 审查请求可包含源码，限制请求体大小以免意外导入过大的文件。
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "请求内容读取失败或超过 32 MiB", nil)
		return false
	}
	if err := jsonfile.Decode(data, value); err != nil {
		writeError(w, http.StatusBadRequest, "JSON 格式错误："+err.Error(), nil)
		return false
	}
	return true
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	status := http.StatusUnprocessableEntity
	code := "invalid_request"
	if errors.Is(err, store.ErrConflict) {
		code = "version_conflict"
		status = http.StatusConflict
	}
	if errors.Is(err, source.ErrStale) {
		code = "source_changed"
		status = http.StatusConflict
	}
	// 临时写锁不等同于版本冲突，浏览器应允许保留编辑直接重试。
	if errors.Is(err, store.ErrLocked) {
		code = "store_locked"
		status = http.StatusLocked
	}
	if errors.Is(err, store.ErrUnconfirmed) {
		code = "unconfirmed"
		status = http.StatusConflict
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code = "timeout"
		status = http.StatusGatewayTimeout
	}
	var incomplete *store.SourceContextError
	if errors.As(err, &incomplete) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		writeJSON(w, map[string]any{"error": err.Error(), "code": "analysis_incomplete", "source": incomplete.Source})
		return
	}
	var validation *design.ValidationError
	var issues []design.Issue
	if errors.As(err, &validation) {
		issues = validation.Issues
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "code": code, "issues": issues})
}

func writeError(w http.ResponseWriter, status int, message string, issues []design.Issue) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": message, "issues": issues})
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}
