// Package store 管理草稿和已确认设计；只有显式 Confirm 可以切换检查基准。
package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"interfaceisall/design"
	"interfaceisall/internal/jsonfile"
	"interfaceisall/internal/projectpath"
)

var (
	ErrUnconfirmed = errors.New("项目尚无已确认设计")
	ErrConflict    = errors.New("设计版本已变化，请重新审阅后确认")
	ErrLocked      = errors.New("设计存储正被写入，请稍后重试")
)

// Snapshot 连同确认记录保存完整设计，Revision 校验整个快照的完整性。
type Snapshot struct {
	Revision       string        `json:"revision"`
	ParentRevision string        `json:"parent_revision"`
	DesignHash     string        `json:"design_hash"`
	ConfirmedAt    time.Time     `json:"confirmed_at"`
	ConfirmedBy    string        `json:"confirmed_by"`
	Design         design.Design `json:"design"`
	// 可选来源不改变旧快照指纹；确认版本保留初次分析证据的永久引用。
	InitialAnalysisID string `json:"initial_analysis_id,omitempty"`
	// 后续重新分析的证据独立记录，保留首次来源和历史版本的兼容性。
	SourceAnalysisID string `json:"source_analysis_id,omitempty"`
}

// Store 只持有项目位置，所有读取均以磁盘当前状态为准。
type Store struct{ root string }

func Open(root string) (*Store, error) {
	root, err := projectpath.Root(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) file(relative string) (string, error) {
	return projectpath.Resolve(s.root, ".architecture/"+relative)
}

// lock 使用排他创建协调多个本地进程；崩溃后的锁不自动抢占。
func (s *Store) lock() (func(), error) {
	name, err := s.file("write.lock")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return nil, ErrLocked
	}
	if err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(f, "pid=%d\n", os.Getpid()); err != nil {
		f.Close()
		os.Remove(name)
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return nil, err
	}
	return func() { _ = os.Remove(name) }, nil
}

// SaveDraft 原子替换草稿，不改变已确认设计。
func (s *Store) SaveDraft(d design.Design) (string, error) {
	return s.saveDraft(d, nil)
}

// SaveDraftIfUnchanged 用同一把跨进程锁检查浏览器读取的草稿与基准，避免覆盖外部工具的修改。
// 空指纹或空版本表示调用方审阅时对应文件尚不存在。
func (s *Store) SaveDraftIfUnchanged(d design.Design, expectedHash, expectedRevision string) (string, error) {
	return s.saveDraft(d, &[2]string{expectedHash, expectedRevision})
}

func (s *Store) saveDraft(d design.Design, expected *[2]string) (string, error) {
	if err := design.ValidateProject(s.root, d); err != nil {
		return "", err
	}
	unlock, err := s.lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	if expected != nil {
		_, hash, err := s.Draft()
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		current, err := s.Confirmed()
		if err != nil && !errors.Is(err, ErrUnconfirmed) {
			return "", err
		}
		if hash != expected[0] || current.Revision != expected[1] {
			return "", ErrConflict
		}
	}
	return s.writeDraft(d)
}

// writeDraft 仅供已持有存储写锁、完成校验的路径调用。
func (s *Store) writeDraft(d design.Design) (string, error) {
	name, err := s.file("draft.json")
	if err != nil {
		return "", err
	}
	if err := jsonfile.Write(name, d); err != nil {
		return "", err
	}
	return design.Fingerprint(d), nil
}

func (s *Store) Draft() (design.Design, string, error) {
	name, err := s.file("draft.json")
	if err != nil {
		return design.Design{}, "", err
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return design.Design{}, "", err
	}
	d, err := design.Parse(data)
	if err != nil {
		return design.Design{}, "", err
	}
	return d, design.Fingerprint(d), nil
}

// Confirm 要求调用方传入已审阅草稿指纹和预期基准；首次确认的基准为空。
// 确认人名称是审计信息，用户审阅由工作台或命令行调用方负责。
func (s *Store) Confirm(draftHash, expectedRevision, actor string) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return s.ConfirmContext(ctx, draftHash, expectedRevision, actor)
}

// ConfirmContext 对本次待确认来源复核源码；已确认历史不再约束日后的手动编辑。
func (s *Store) ConfirmContext(ctx context.Context, draftHash, expectedRevision, actor string) (Snapshot, error) {
	if strings.TrimSpace(actor) == "" || draftHash == "" {
		return Snapshot{}, fmt.Errorf("必须提供确认人和已审阅的草稿指纹")
	}
	unlock, err := s.lock()
	if err != nil {
		return Snapshot{}, err
	}
	defer unlock()
	d, actualHash, err := s.Draft()
	if err != nil {
		return Snapshot{}, err
	}
	if actualHash != draftHash {
		return Snapshot{}, ErrConflict
	}
	if err := design.ValidateProject(s.root, d); err != nil {
		return Snapshot{}, err
	}
	current, err := s.Confirmed()
	if err != nil && !errors.Is(err, ErrUnconfirmed) {
		return Snapshot{}, err
	}
	if current.Revision != expectedRevision {
		return Snapshot{}, ErrConflict
	}
	initialID := current.InitialAnalysisID
	sourceID := current.SourceAnalysisID
	initial, err := s.InitialAnalysis()
	if err != nil {
		return Snapshot{}, err
	}
	if initial != nil && !initial.Confirmed {
		if initial.Withdrawn {
			return Snapshot{}, fmt.Errorf("已撤回源码分析，请重新分析并接受新提案后再确认；原草稿和基准仍保留")
		}
		if !initial.Accepted {
			return Snapshot{}, fmt.Errorf("请先接受或放弃待审阅的源码分析提案")
		}
		p, err := s.Proposal(initial.ProposalID)
		if err != nil {
			return Snapshot{}, err
		}
		if p.ExpectedRevision != current.Revision {
			return Snapshot{}, ErrConflict
		}
		if err := p.Request.Source.Verify(ctx, s.root); err != nil {
			return Snapshot{}, err
		}
		if current.Revision == "" {
			initialID = p.ID
		} else {
			sourceID = p.ID
		}
	}
	// 即使设计文字不变，新源码分析也要发布新证据，不能被幂等返回吞掉。
	if current.DesignHash == actualHash && initialID == current.InitialAnalysisID && sourceID == current.SourceAnalysisID {
		return current, nil
	}
	snapshot := Snapshot{
		ParentRevision: current.Revision, DesignHash: actualHash,
		ConfirmedAt: time.Now().UTC(), ConfirmedBy: actor, Design: d,
		InitialAnalysisID: initialID,
		SourceAnalysisID:  sourceID,
	}
	snapshot.Revision = snapshotHash(snapshot)
	archive, err := s.file("versions/" + snapshot.Revision + ".json")
	if err != nil {
		return Snapshot{}, err
	}
	// 先持久化历史，再发布当前版本；发布失败最多留下未引用的完整历史。
	if _, err := os.Lstat(archive); !os.IsNotExist(err) {
		return Snapshot{}, fmt.Errorf("历史版本路径已存在或不可用：%s", archive)
	}
	if err := jsonfile.Write(archive, snapshot); err != nil {
		return Snapshot{}, err
	}
	name, err := s.file("confirmed.json")
	if err != nil {
		return Snapshot{}, err
	}
	if err := jsonfile.Write(name, snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// Confirmed 拒绝损坏、手工改写或缺少历史文件的基准，防止静默使用错误约束。
func (s *Store) Confirmed() (Snapshot, error) {
	name, err := s.file("confirmed.json")
	if err != nil {
		return Snapshot{}, err
	}
	data, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return Snapshot{}, ErrUnconfirmed
	}
	if err != nil {
		return Snapshot{}, err
	}
	var snapshot Snapshot
	if err := jsonfile.Decode(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	archive, err := s.file("versions/" + snapshot.Revision + ".json")
	if err != nil {
		return Snapshot{}, err
	}
	archived, err := os.ReadFile(archive)
	if err != nil {
		return Snapshot{}, fmt.Errorf("无法读取已确认设计的历史版本：%w", err)
	}
	var historical Snapshot
	if err := jsonfile.Decode(archived, &historical); err != nil {
		return Snapshot{}, err
	}
	if err := historical.Validate(); err != nil || historical.Revision != snapshot.Revision {
		return Snapshot{}, fmt.Errorf("当前设计与历史版本不一致")
	}
	return snapshot, nil
}

// Validate 也用于验证审查请求附带的设计基准，不能只信任自报的版本号。
func (s Snapshot) Validate() error {
	if err := design.Validate(s.Design); err != nil {
		return err
	}
	if s.ConfirmedAt.IsZero() || strings.TrimSpace(s.ConfirmedBy) == "" || s.DesignHash != design.Fingerprint(s.Design) || s.Revision != snapshotHash(s) {
		return fmt.Errorf("已确认设计快照的确认记录或内容指纹无效")
	}
	return nil
}

func snapshotHash(s Snapshot) string {
	s.Revision = ""
	data, _ := json.Marshal(s)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
