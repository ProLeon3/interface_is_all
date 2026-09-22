package workbench

import "net/http"

func (s *Server) discardInitial(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID               string  `json:"proposal_id"`
		ExpectedHash     *string `json:"expected_draft_hash"`
		ExpectedRevision *string `json:"expected_revision"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.ExpectedHash == nil || input.ExpectedRevision == nil {
		writeError(w, 400, "放弃分析必须提供当前版本", nil)
		return
	}
	if err := s.store.DiscardInitialAnalysis(input.ID, *input.ExpectedHash, *input.ExpectedRevision); err != nil {
		s.fail(w, err)
		return
	}
	initial, err := s.store.InitialAnalysis()
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, map[string]any{"discarded": true, "initial_analysis": initial})
}
