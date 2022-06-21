package main

import (
	"fmt"
	"github.com/opensourceways/community-robot-lib/gitlabclient"
	"github.com/xanzy/go-gitlab"
	"k8s.io/apimachinery/pkg/util/sets"
	"strings"
)

const (
	retestCommand     = "/retest"
	msgNotSetReviewer = "**@%s** Thank you for submitting a PullRequest. It is detected that you have not set a reviewer, please set a one."
)

func (bot *robot) doRetest(e *gitlab.MergeEvent) error {
	if e.ObjectAttributes.State != "opened" || !gitlabclient.CheckSourceBranchChanged(e) {
		return nil
	}

	pid := e.Project.ID
	mrID := e.ObjectAttributes.IID

	return bot.cli.CreateMergeRequestComment(pid, mrID, retestCommand)
}

func (bot *robot) checkReviewer(e *gitlab.MergeEvent, cfg *botConfig) error {
	if cfg.UnableCheckingReviewerForPR || e.ObjectAttributes.State != "opened" {
		return nil
	}

	if e != nil && len(e.ObjectAttributes.AssigneeIDs) > 0 {
		return nil
	}

	pid := e.Project.ID
	mrID := e.ObjectAttributes.IID
	author := gitlabclient.GetMRAuthor(e)

	return bot.cli.CreateMergeRequestComment(
		pid, mrID,
		fmt.Sprintf(msgNotSetReviewer, author),
	)
}

func (bot *robot) clearLabel(e *gitlab.MergeEvent) error {
	if e.ObjectAttributes.State != "opened" || !gitlabclient.CheckSourceBranchChanged(e) {
		return nil
	}

	pid := e.Project.ID
	mrID := e.ObjectAttributes.IID
	labelSet := sets.NewString()
	mrLabels, err := bot.cli.GetMergeRequestLabels(pid, mrID)
	if err != nil {
		return err
	}
	labelSet.Insert(mrLabels...)
	v := getLGTMLabelsOnPR(labelSet)

	if labelSet.Has(approvedLabel) {
		v = append(v, approvedLabel)
	}

	if len(v) > 0 {

		if err := bot.cli.RemoveMergeRequestLabel(pid, mrID, v); err != nil {
			return err
		}

		return bot.cli.CreateMergeRequestComment(
			pid, mrID,
			fmt.Sprintf(commentClearLabel, strings.Join(v, ", ")),
		)
	}

	return nil
}
