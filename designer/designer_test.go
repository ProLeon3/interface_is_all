package designer

import (
	"encoding/json"
	"interfaceisall/source"
	"strings"
	"testing"
)

// 外部工具必须遵守结果结构，不能以模型格式或重复字段绕过校验。
func TestExternalResponseRejectsInvalidJSONAndDesign(t *testing.T) {
	data, _ := json.Marshal(Response{RequestID: strings.Repeat("a", 64), Result: Result{Summary: "空项目草稿", Design: source.EmptyDesign()}})
	if _, err := ParseResponse(data, Request{}); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		strings.Replace(string(data), `"summary":"空项目草稿"`, `"summary":""`, 1),
		strings.Replace(string(data), `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(string(data), `"summary":`, `"summary":"重复", "summary":`, 1),
		string(data) + " {}",
	} {
		if _, err := ParseResponse([]byte(body), Request{}); err == nil {
			t.Fatal("接受了无效结果", body)
		}
	}
}
