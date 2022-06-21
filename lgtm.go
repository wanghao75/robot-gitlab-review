package main

import (
	"fmt"
	"github.com/opensourceways/community-robot-lib/gitlabclient"
	"github.com/xanzy/go-gitlab"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// the gitee platform limits the maximum length of label to 20.
	labelLenLimit = 20
	lgtmLabel     = "lgtm"

	commentAddLGTMBySelf            = "***lgtm*** can not be added in your self-own pull request. :astonished:"
	commentClearLabel               = `New code changes of pr are detected and remove these labels ***%s***. :flushed: `
	commentNoPermissionForLgtmLabel = `Thanks for your review, ***%s***, your opinion is very important to us.:wave:
The maintainers will consider your advice carefully.`
	commentNoPermissionForLabel = `
***@%s*** has no permission to %s ***%s*** label in this pull request. :astonished:
Please contact to the collaborators in this repository.`
	commentAddLabel = `***%s*** was added to this pull request by: ***%s***. :wave: 
**NOTE:** If this pull request is not merged while all conditions are met, comment "/check-pr" to try again. :smile: `
	commentRemovedLabel = `***%s*** was removed in this pull request by: ***%s***. :flushed: `
)

var (
	regAddLgtm    = regexp.MustCompile(`(?mi)^/lgtm\s*$`)
	regRemoveLgtm = regexp.MustCompile(`(?mi)^/lgtm cancel\s*$`)
)

func (bot *robot) handleLGTM(e *gitlab.MergeCommentEvent, cfg *botConfig, log *logrus.Entry) error {
	if e.MergeRequest.State != "opened" || e.ObjectKind != "note" {
		return nil
	}

	comment := gitlabclient.GetMRCommentBody(e)

	if regAddLgtm.MatchString(comment) {
		return bot.addLGTM(cfg, e, log)
	}

	if regRemoveLgtm.MatchString(comment) {
		return bot.removeLGTM(cfg, e, log)
	}

	return nil
}

func (bot *robot) addLGTM(cfg *botConfig, e *gitlab.MergeCommentEvent, log *logrus.Entry) error {
	org, repo := gitlabclient.GetMRCommentOrgAndRepo(e)
	number := e.MergeRequest.IID
	pid := e.ProjectID
	commenterID := gitlabclient.GetMRCommentAuthorID(e)
	commenter := gitlabclient.GetMRCommentAuthor(e)
	mrAuthorID := e.MergeRequest.AuthorID

	if mrAuthorID == commenterID {
		return bot.cli.CreateMergeRequestComment(pid, number, commentAddLGTMBySelf)
	}

	v, err := bot.hasPermission(
		org, repo, commenter, commenterID, cfg.CheckPermissionBasedOnSigOwners, e, cfg, log,
	)
	if err != nil {
		return err
	}
	if !v {
		return bot.cli.CreateMergeRequestComment(
			pid, number,
			fmt.Sprintf(commentNoPermissionForLgtmLabel, commenter),
		)
	}

	label := genLGTMLabel(commenter, cfg.LgtmCountsRequired)
	if label != lgtmLabel {
		if err := bot.createLabelIfNeed(pid, label); err != nil {
			log.WithError(err).Errorf("create repo label: %s", label)
		}
	}

	if err := bot.cli.AddMergeRequestLabel(pid, number, []string{label}); err != nil {
		return err
	}

	err = bot.cli.CreateMergeRequestComment(
		pid, number, fmt.Sprintf(commentAddLabel, label, commenter),
	)
	if err != nil {
		log.Error(err)
	}

	return bot.tryMerge(e, cfg, false, log)
}

func (bot *robot) removeLGTM(cfg *botConfig, e *gitlab.MergeCommentEvent, log *logrus.Entry) error {
	org, repo := gitlabclient.GetMRCommentOrgAndRepo(e)
	number := e.MergeRequest.IID
	pid := e.ProjectID
	commenterID := gitlabclient.GetMRCommentAuthorID(e)
	commenter := gitlabclient.GetMRCommentAuthor(e)
	mrAuthorID := e.MergeRequest.AuthorID

	if mrAuthorID != commenterID {
		v, err := bot.hasPermission(
			org, repo, commenter, commenterID, cfg.CheckPermissionBasedOnSigOwners, e, cfg, log,
		)
		if err != nil {
			return err
		}
		if !v {
			return bot.cli.CreateMergeRequestComment(pid, number, fmt.Sprintf(
				commentNoPermissionForLabel, commenter, "remove", lgtmLabel,
			))
		}

		l := genLGTMLabel(commenter, cfg.LgtmCountsRequired)
		if err = bot.cli.RemoveMergeRequestLabel(pid, number, []string{l}); err != nil {
			return err
		}

		return bot.cli.CreateMergeRequestComment(
			pid, number, fmt.Sprintf(commentRemovedLabel, l, commenter),
		)
	}

	// the author of pr can remove all of lgtm[-login name] kind labels
	lbs := sets.NewString()
	mrLabels, err := bot.cli.GetMergeRequestLabels(pid, number)
	if err != nil {
		return err
	}
	lbs.Insert(mrLabels...)
	if v := getLGTMLabelsOnPR(lbs); len(v) > 0 {
		return bot.cli.RemoveMergeRequestLabel(pid, number, v)
	}

	return nil
}

func (bot *robot) createLabelIfNeed(pid int, label string) error {
	repoLabels, err := bot.cli.GetProjectLabels(pid)
	if err != nil {
		return err
	}

	for _, v := range repoLabels {
		if v.Name == label {
			return nil
		}
	}

	return bot.cli.CreateProjectLabel(pid, label, "")
}

func genLGTMLabel(commenter string, lgtmCount uint) string {
	if lgtmCount <= 1 {
		return lgtmLabel
	}

	l := fmt.Sprintf("%s-%s", lgtmLabel, strings.ToLower(commenter))
	if len(l) > labelLenLimit {
		return l[:labelLenLimit]
	}

	return l
}

func getLGTMLabelsOnPR(labels sets.String) []string {
	var r []string

	for l := range labels {
		if strings.HasPrefix(l, lgtmLabel) {
			r = append(r, l)
		}
	}

	return r
}
