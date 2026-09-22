package main

import (
	"context"
	"fmt"
	"io"

	"github.com/ProLeon3/interface_is_all/check"
	"github.com/ProLeon3/interface_is_all/store"
)

func runCheck(ctx context.Context, s *store.Store, command, project, tags, requestFile, responseFile string, write func(any) int, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, err)
		return 2
	}
	current, err := s.Confirmed()
	if err != nil {
		return fail(err)
	}
	if command == "review-import" {
		requestData, err := readInput(requestFile)
		if err != nil {
			return fail(err)
		}
		request, err := check.ParseReviewRequest(requestData)
		if err != nil {
			return fail(err)
		}
		responseData, err := readInput(responseFile)
		if err != nil {
			return fail(err)
		}
		response, err := check.ParseReviewResponse(responseData)
		if err != nil {
			return fail(err)
		}
		report, err := check.ImportReview(ctx, project, current, request, response)
		if err != nil {
			return fail(err)
		}
		path, err := s.SaveArtifact("review", current.Revision, report)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(stderr, "能力审查报告已保存："+path)
		return write(report)
	}
	report, err := check.Run(ctx, project, current, check.Options{Tags: tags})
	if err != nil {
		return fail(err)
	}
	path, err := s.SaveArtifact("check", current.Revision, report)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintln(stderr, "确定性检查报告已保存："+path)
	if command == "review-request" {
		request, err := check.NewReviewRequest(current, report.Facts)
		if err != nil {
			write(report)
			return fail(err)
		}
		path, err := s.SaveArtifact("review-request", current.Revision, request)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(stderr, "能力审查请求已保存："+path)
		if code := write(request); code != 0 {
			return code
		}
	} else if code := write(report); code != 0 {
		return code
	}
	// 即使成功生成审查请求，仍通过退出码报告本次已发现的确定性问题。
	if report.Deterministic.Status == "incomplete" {
		return 2
	}
	if len(report.Deterministic.Violations) > 0 {
		return 1
	}
	return 0
}
