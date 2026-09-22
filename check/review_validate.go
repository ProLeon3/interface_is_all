package check

import (
	"encoding/json"
	"fmt"
	"strings"

	"interfaceisall/design"
)

// ValidateReview 核对接口覆盖、模块归属和证据位置，不将语义推断升级成确定性结论。
// 离线调用只验证所给快照；接收当前项目的结果应使用 ImportReview。
func ValidateReview(request ReviewRequest, response ReviewResponse) (SemanticReport, error) {
	requestData, err := json.Marshal(request)
	if err != nil {
		return SemanticReport{}, err
	}
	if _, err := ParseReviewRequest(requestData); err != nil {
		return SemanticReport{}, err
	}
	data, err := json.Marshal(response)
	if err != nil {
		return SemanticReport{}, err
	}
	if _, err := ParseReviewResponse(data); err != nil {
		return SemanticReport{}, err
	}
	if response.RequestID != request.RequestID {
		return SemanticReport{}, fmt.Errorf("审查响应与请求不匹配")
	}
	modules := map[string]design.Module{}
	interfaces := map[string]design.Interface{}
	entries := map[string]Entry{}
	files := map[string]SourceFile{}
	for _, module := range request.Baseline.Design.Modules {
		modules[module.ID] = module
	}
	for _, api := range request.Baseline.Design.Interfaces {
		interfaces[api.ID] = api
	}
	for _, entry := range request.Facts.Entries {
		if _, exists := entries[entry.ID]; exists {
			return SemanticReport{}, fmt.Errorf("代码存在重复入口标识：%s", entry.ID)
		}
		entries[entry.ID] = entry
	}
	for _, file := range request.Facts.Files {
		files[file.Path] = file
	}
	validateEvidence := func(moduleID string, entryIDs []string, evidence []Evidence) error {
		for _, id := range entryIDs {
			entry, exists := entries[id]
			if !exists || entry.ModuleID != moduleID {
				return fmt.Errorf("入口不存在或不属于模块 %s：%s", moduleID, id)
			}
		}
		for _, item := range evidence {
			file, exists := files[item.File]
			if !exists || file.ModuleID != moduleID {
				return fmt.Errorf("证据文件不存在或不属于模块 %s：%s", moduleID, item.File)
			}
			lines := strings.Count(strings.TrimSuffix(file.Content, "\n"), "\n") + 1
			if item.Line < 1 || item.EndLine < item.Line || item.EndLine > lines {
				return fmt.Errorf("证据行号超出有效范围：%s:%d-%d", item.File, item.Line, item.EndLine)
			}
		}
		if len(entryIDs) > 0 && len(evidence) == 0 {
			return fmt.Errorf("实现关联必须提供代码证据")
		}
		return nil
	}
	result := SemanticReport{
		SchemaVersion: 1, RequestID: request.RequestID, DesignRevision: request.Baseline.Revision,
		Status: "pending_confirmation", Interfaces: []InterfaceReview{}, Concerns: []ConcernReview{},
	}
	seen := map[string]bool{}
	for _, association := range response.Interfaces {
		api, exists := interfaces[association.InterfaceID]
		if !exists || seen[association.InterfaceID] {
			return SemanticReport{}, fmt.Errorf("接口不存在或被重复审查：%s", association.InterfaceID)
		}
		seen[api.ID] = true
		if association.Assessment == "not_found" && len(association.EntryIDs) > 0 {
			return SemanticReport{}, fmt.Errorf("尚未找到实现时不能同时关联入口：%s", api.ID)
		}
		if (association.Assessment == "aligned" || association.Assessment == "potential_deviation") && len(association.EntryIDs) == 0 {
			return SemanticReport{}, fmt.Errorf("符合设计或潜在偏离的判断必须关联实现入口：%s", api.ID)
		}
		if err := validateEvidence(api.ModuleID, association.EntryIDs, association.Evidence); err != nil {
			return SemanticReport{}, fmt.Errorf("接口 %s：%w", api.ID, err)
		}
		searched := []string{}
		for _, file := range request.Facts.Files {
			if file.ModuleID == api.ModuleID {
				searched = append(searched, file.Path)
			}
		}
		result.Interfaces = append(result.Interfaces, InterfaceReview{"pending_confirmation", api, searched, association})
	}
	if len(seen) != len(interfaces) {
		return SemanticReport{}, fmt.Errorf("审查响应未覆盖全部设计接口：需要 %d 项，实际 %d 项", len(interfaces), len(seen))
	}
	for _, concern := range response.Concerns {
		module, exists := modules[concern.ModuleID]
		if !exists {
			return SemanticReport{}, fmt.Errorf("潜在偏离引用了未知模块：%s", concern.ModuleID)
		}
		if err := validateEvidence(module.ID, concern.EntryIDs, concern.Evidence); err != nil {
			return SemanticReport{}, err
		}
		result.Concerns = append(result.Concerns, ConcernReview{"pending_confirmation", module, concern})
	}
	return result, nil
}
