package store_test

import (
	"errors"
	"testing"

	"github.com/ProLeon3/interface_is_all/design"
	"github.com/ProLeon3/interface_is_all/internal/testproject"
	"github.com/ProLeon3/interface_is_all/store"
)

func TestConcurrentProposalAcceptanceCannotOverwriteBaseline(t *testing.T) {
	_, s, baseline := testproject.New(t)
	after := testproject.Design()
	after.Modules[0].Responsibility = "并发接受的新职责"
	p, err := s.SaveProposal(store.Proposal{Before: baseline.Design, After: after, Summary: "调整职责", ExpectedHash: baseline.DesignHash, ExpectedRevision: baseline.Revision})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	// 两个窗口同时接受同一份有实质改动的提案，只有一个能写入草稿。
	for i := 0; i < 2; i++ {
		go func() { <-start; _, err := s.AcceptProposal(p.ID); results <- err }()
	}
	close(start)
	succeeded := 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			succeeded++
		} else if !errors.Is(err, store.ErrConflict) && !errors.Is(err, store.ErrLocked) {
			t.Fatal(err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("接受成功次数：%d", succeeded)
	}
	_, hash, err := s.Draft()
	if err != nil || hash != design.Fingerprint(after) {
		t.Fatal("草稿不完整", err)
	}
	current, err := s.Confirmed()
	if err != nil || current.Revision != baseline.Revision {
		t.Fatal("接受改变了确认版本", err)
	}
}
