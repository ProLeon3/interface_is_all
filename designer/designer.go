// Package designer 定义外部设计产物及确定性校验，不执行模型调用。
package designer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ProLeon3/interface_is_all/design"
	"github.com/ProLeon3/interface_is_all/internal/jsonfile"
	"github.com/ProLeon3/interface_is_all/source"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// Selection 保留用户实际点选的对象类型和稳定 ID，不能让模型猜测上下文。
type Selection struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func (s Selection) Validate(d design.Design) error {
	if s.Kind == "design" && s.ID == "" {
		return nil
	}
	if s.Kind == "module" {
		for _, m := range d.Modules {
			if m.ID == s.ID {
				return nil
			}
		}
	}
	if s.Kind == "interface" {
		for _, a := range d.Interfaces {
			if a.ID == s.ID {
				return nil
			}
		}
	}
	if s.Kind == "collaboration" {
		for _, c := range d.Collaborations {
			if c.ID == s.ID {
				return nil
			}
		}
	}
	return fmt.Errorf("选中的设计对象不存在，请重新选择")
}

// Request 的 Source 由首次或重新分析及其后续调整填充；普通需求设计不读取源码。
type Request struct {
	Requirement string          `json:"requirement"`
	Instruction string          `json:"instruction"`
	Selection   Selection       `json:"selection"`
	Design      design.Design   `json:"design"`
	Source      *source.Context `json:"source,omitempty"`
}

type Result struct {
	Summary  string        `json:"summary"`
	Design   design.Design `json:"design"`
	Analysis *Analysis     `json:"analysis,omitempty"`
}

func responseSchema() map[string]any {
	var body map[string]any
	_ = json.Unmarshal([]byte(design.Schema), &body)
	defs := body["$defs"].(map[string]any)
	delete(body, "$defs")
	delete(body, "$schema")
	delete(body, "$id")
	body["properties"].(map[string]any)["schema_version"] = map[string]any{"type": "integer", "enum": []int{1}}
	defs["design"] = body
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"summary", "design"}, "properties": map[string]any{"summary": map[string]any{"type": "string", "minLength": 1}, "design": map[string]string{"$ref": "#/$defs/design"}}, "$defs": defs}
	return schema
}

// Response 绑定由程序保存的请求，外部 agent 只提交结果而不能改写输入依据。
type Response struct {
	RequestID string `json:"request_id"`
	Result
}

// ResponseSchema 导出工具无关的结果格式，不包含模型供应商参数。
func ResponseSchema(withSource bool) json.RawMessage {
	schema := responseSchema()
	if withSource {
		schema = analysisResponseSchema(schema)
	}
	schema["properties"].(map[string]any)["request_id"] = map[string]any{"type": "string", "pattern": "^[a-f0-9]{64}$"}
	schema["required"] = append(schema["required"].([]string), "request_id")
	data, _ := json.Marshal(schema)
	return data
}

// ParseResponse 同时核对 JSON 结构、设计关系与源码依据；语义结论仍须用户审阅。
func ParseResponse(data []byte, input Request) (Response, error) {
	var raw any
	if err := jsonfile.Decode(data, &raw); err != nil {
		return Response{}, err
	}
	schema, err := jsonschema.CompileString("proposal-response.json", string(ResponseSchema(input.Source != nil)))
	if err != nil {
		return Response{}, err
	}
	if err := schema.Validate(raw); err != nil {
		return Response{}, fmt.Errorf("提案响应结构无效：%w", err)
	}
	var response Response
	if err := jsonfile.Decode(data, &response); err != nil {
		return Response{}, err
	}
	if err := design.Validate(response.Design); err != nil {
		return Response{}, err
	}
	if strings.TrimSpace(response.Summary) == "" {
		return Response{}, fmt.Errorf("提案缺少变更说明")
	}
	if input.Source != nil {
		completeCollaborationEvidence(&response.Result)
		if err := ValidateAnalysis(*input.Source, response.Result); err != nil {
			return Response{}, err
		}
	}
	return response, nil
}
