package main

import (
	"fmt"
	"github.com/opensourceways/community-robot-lib/gitlabclient"
	"github.com/xanzy/go-gitlab"
	"regexp"

	"github.com/sirupsen/logrus"
)

const approvedLabel = "approved"

var (
	regAddApprove    = regexp.MustCompile(`(?mi)^/approved\s*$`)
	regRemoveApprove = regexp.MustCompile(`(?mi)^/approved cancel\s*$`)
)

func (bot *robot) handleApprove(e *gitlab.MergeCommentEvent, cfg *botConfig, log *logrus.Entry) error {
	if e.MergeRequest.State != gitlabclient.ActionOpened || e.ObjectKind != "note" {
		return nil
	}

	comment := gitlabclient.GetMRCommentBody(e)
	if regAddApprove.MatchString(comment) {
		return bot.AddApprove(cfg, e, log)
	}

	if regRemoveApprove.MatchString(comment) {
		return bot.removeApprove(cfg, e, log)
	}

	return nil
}

func (bot *robot) AddApprove(cfg *botConfig, e *gitlab.MergeCommentEvent, log *logrus.Entry) error {
	org, repo := gitlabclient.GetMRCommentOrgAndRepo(e)
	commenter := gitlabclient.GetMRCommentAuthor(e)
	commenterID := gitlabclient.GetMRCommentAuthorID(e)
	number := e.MergeRequest.IID
	pid := e.ProjectID

	v, err := bot.hasPermission(org, repo, commenter, commenterID, false, e, cfg, log)
	if err != nil {
		return err
	}

	if !v {
		return bot.cli.CreateMergeRequestComment(pid, number, fmt.Sprintf(
			commentNoPermissionForLabel, commenter, "add", approvedLabel,
		))
	}

	if err := bot.cli.AddMergeRequestLabel(pid, number, []string{approvedLabel}); err != nil {
		return err
	}

	err = bot.cli.CreateMergeRequestComment(
		pid, number,
		fmt.Sprintf(commentAddLabel, approvedLabel, commenter),
	)
	if err != nil {
		log.Error(err)
	}

	return bot.tryMerge(e, cfg, false, log)
}

func (bot *robot) removeApprove(cfg *botConfig, e *gitlab.MergeCommentEvent, log *logrus.Entry) error {
	org, repo := gitlabclient.GetMRCommentOrgAndRepo(e)
	commenter := gitlabclient.GetMRCommentAuthor(e)
	commenterID := gitlabclient.GetMRCommentAuthorID(e)
	number := e.MergeRequest.IID
	pid := e.ProjectID

	v, err := bot.hasPermission(org, repo, commenter, commenterID, false, e, cfg, log)
	if err != nil {
		return err
	}

	if !v {
		return bot.cli.CreateMergeRequestComment(pid, number, fmt.Sprintf(
			commentNoPermissionForLabel, commenter, "remove", approvedLabel,
		))
	}

	err = bot.cli.RemoveMergeRequestLabel(pid, number, []string{approvedLabel})
	if err != nil {
		return err
	}

	return bot.cli.CreateMergeRequestComment(
		pid, number,
		fmt.Sprintf(commentRemovedLabel, approvedLabel, commenter),
	)
}
