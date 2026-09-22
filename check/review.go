package check

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"interfaceisall/design"
	"interfaceisall/internal/jsonfile"
	"interfaceisall/store"
)

// 这里只描述交换约定；外部 agent 的审查提示词独立放在 docs/agent-prompts。
const reviewInstructions = "逐项返回接口关联和源码依据，结果保持待确认；输出须符合 response_schema。"

//go:embed review.schema.json
var reviewSchema string

var compiledReviewSchema = jsonschema.MustCompileString("review.schema.json", reviewSchema)

// ReviewRequest 是与现有 AI 工具交换的自包含输入，不绑定某个模型供应商。
type ReviewRequest struct {
	SchemaVersion  int             `json:"schema_version"`
	RequestID      string          `json:"request_id"`
	Instructions   string          `json:"instructions"`
	ResponseSchema json.RawMessage `json:"response_schema"`
	Baseline       store.Snapshot  `json:"baseline"`
	Facts          Facts           `json:"facts"`
}

type Evidence struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	EndLine int    `json:"end_line"`
}

type Association struct {
	InterfaceID string     `json:"interface_id"`
	EntryIDs    []string   `json:"entry_ids"`
	Assessment  string     `json:"assessment"`
	Reason      string     `json:"reason"`
	Evidence    []Evidence `json:"evidence"`
}

type Concern struct {
	Kind     string     `json:"kind"`
	ModuleID string     `json:"module_id"`
	EntryIDs []string   `json:"entry_ids"`
	Reason   string     `json:"reason"`
	Evidence []Evidence `json:"evidence"`
}

type ReviewResponse struct {
	SchemaVersion int           `json:"schema_version"`
	RequestID     string        `json:"request_id"`
	Interfaces    []Association `json:"interfaces"`
	Concerns      []Concern     `json:"concerns"`
}

// InterfaceReview 附加程序从基准读取的原始设计，不允许模型伪造设计依据。
type InterfaceReview struct {
	Status        string           `json:"status"`
	DesignBasis   design.Interface `json:"design_basis"`
	SearchedFiles []string         `json:"searched_files"`
	Conclusion    Association      `json:"conclusion"`
}

type ConcernReview struct {
	Status      string        `json:"status"`
	DesignBasis design.Module `json:"design_basis"`
	Conclusion  Concern       `json:"conclusion"`
}

type SemanticReport struct {
	SchemaVersion  int               `json:"schema_version"`
	RequestID      string            `json:"request_id"`
	DesignRevision string            `json:"design_revision"`
	Status         string            `json:"status"`
	Interfaces     []InterfaceReview `json:"interfaces"`
	Concerns       []ConcernReview   `json:"concerns"`
}

// NewReviewRequest 对完整扫描生成内容指纹，关联设计、构建条件和代码快照。
func NewReviewRequest(baseline store.Snapshot, facts Facts) (ReviewRequest, error) {
	if err := baseline.Validate(); err != nil {
		return ReviewRequest{}, err
	}
	if len(facts.Diagnostics) > 0 {
		return ReviewRequest{}, fmt.Errorf("代码扫描不完整，不能生成完整能力审查请求")
	}
	request := ReviewRequest{1, "", reviewInstructions, json.RawMessage(reviewSchema), baseline, facts}
	request.RequestID = requestHash(request)
	return request, nil
}

func requestHash(request ReviewRequest) string {
	request.RequestID = ""
	data, _ := json.Marshal(request)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func ParseReviewRequest(data []byte) (ReviewRequest, error) {
	var request ReviewRequest
	if err := jsonfile.Decode(data, &request); err != nil {
		return request, err
	}
	if err := request.Baseline.Validate(); err != nil {
		return request, err
	}
	if request.SchemaVersion != 1 || request.RequestID != requestHash(request) || len(request.Facts.Diagnostics) > 0 {
		return request, fmt.Errorf("审查请求的版本、完整性或代码覆盖无效")
	}
	return request, nil
}

func ParseReviewResponse(data []byte) (ReviewResponse, error) {
	var raw any
	if err := jsonfile.Decode(data, &raw); err != nil {
		return ReviewResponse{}, err
	}
	if err := compiledReviewSchema.Validate(raw); err != nil {
		return ReviewResponse{}, fmt.Errorf("能力审查响应结构无效：%w", err)
	}
	var response ReviewResponse
	err := jsonfile.Decode(data, &response)
	return response, err
}

// ImportReview 重新扫描当前代码，拒绝已过期的代码证据或设计版本。
// 返回报告后由调用方保存为观察结果；此路径不会确认或修改设计。
func ImportReview(ctx context.Context, root string, current store.Snapshot, request ReviewRequest, response ReviewResponse) (SemanticReport, error) {
	if request.Baseline.Revision != current.Revision {
		return SemanticReport{}, fmt.Errorf("已确认设计发生变化，请重新发起能力审查")
	}
	facts, err := Scan(ctx, root, current.Design, Options{Tags: request.Facts.Scope.Tags})
	if err != nil {
		return SemanticReport{}, err
	}
	fresh, err := NewReviewRequest(current, facts)
	if err != nil {
		return SemanticReport{}, err
	}
	if fresh.RequestID != request.RequestID || request.RequestID != requestHash(request) {
		return SemanticReport{}, fmt.Errorf("源码、构建条件或审查请求已变化，请重新发起能力审查")
	}
	return ValidateReview(fresh, response)
}
