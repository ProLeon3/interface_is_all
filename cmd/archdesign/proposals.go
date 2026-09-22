package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ProLeon3/interface_is_all/internal/jsonfile"
	"github.com/ProLeon3/interface_is_all/store"
)

// 外部 agent 使用文件交换；所有写入复用工作台的校验、锁和确认边界。
func runProposal(ctx context.Context, s *store.Store, command, kind, file, id, tags string, write func(any) int, stderr io.Writer) int {
	fail := func(err error) int { fmt.Fprintln(stderr, err); return 2 }
	switch command {
	case "design-request":
		var input store.DesignRequestOptions
		if file != "" {
			data, err := readInput(file)
			if err != nil {
				return fail(err)
			}
			if err := jsonfile.Decode(data, &input); err != nil {
				return fail(err)
			}
		}
		if kind != "" {
			input.Kind = kind
		}
		request, err := s.PrepareDesignRequest(ctx, input, tags)
		if err != nil {
			var incomplete *store.SourceContextError
			if errors.As(err, &incomplete) {
				write(map[string]any{"error": err.Error(), "source": incomplete.Source})
			}
			return fail(err)
		}
		return write(request)
	case "proposal-import":
		data, err := readInput(file)
		if err != nil {
			return fail(err)
		}
		p, err := s.ImportProposal(ctx, data)
		if err != nil {
			return fail(err)
		}
		return write(p)
	case "proposal-show":
		if id == "" {
			initial, err := s.InitialAnalysis()
			if err != nil {
				return fail(err)
			}
			if initial != nil && !initial.Accepted && !initial.Withdrawn && !initial.Confirmed {
				id = initial.ProposalID
			}
			if id == "" {
				var err error
				id, err = s.PendingDesignProposal()
				if err != nil {
					return fail(err)
				}
			}
		}
		if id == "" {
			return fail(fmt.Errorf("没有待审阅提案；读取历史提案请提供 -id"))
		}
		p, err := s.Proposal(id)
		if err != nil {
			return fail(err)
		}
		return write(p)
	case "proposal-accept":
		hash, err := s.AcceptProposalContext(ctx, id)
		if err != nil {
			return fail(err)
		}
		return write(map[string]string{"draft_hash": hash})
	case "proposal-discard":
		p, err := s.Proposal(id)
		if err != nil {
			return fail(err)
		}
		if p.Request.Source == nil {
			err = s.DiscardDesignProposal(id)
		} else {
			_, hash, readErr := s.Draft()
			if readErr != nil && !os.IsNotExist(readErr) {
				return fail(readErr)
			}
			current, readErr := s.Confirmed()
			if readErr != nil && !errors.Is(readErr, store.ErrUnconfirmed) {
				return fail(readErr)
			}
			err = s.DiscardInitialAnalysis(id, hash, current.Revision)
		}
		if err != nil {
			return fail(err)
		}
		return write(map[string]bool{"discarded": true})
	}
	return fail(fmt.Errorf("未知提案命令"))
}
