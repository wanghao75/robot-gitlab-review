package main

import (
	"encoding/base64"
	"fmt"
	"github.com/xanzy/go-gitlab"
	"path/filepath"
	"strings"

	"github.com/opensourceways/repo-file-cache/models"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"
)

const ownerFile = "OWNERS"
const sigInfoFile = "sig-info.yaml"

func (bot *robot) hasPermission(
	org, repo, commenter string,
	commenterID int,
	needCheckSig bool,
	e *gitlab.MergeCommentEvent,
	cfg *botConfig,
	log *logrus.Entry,
) (bool, error) {
	commenter = strings.ToLower(commenter)
	hasPermission, err := bot.cli.GetUserPermissionOfProject(e.ProjectID, commenterID)
	if err != nil {
		return false, err
	}

	if hasPermission {
		return true, nil
	}

	if needCheckSig {
		return bot.isOwnerOfSig(org, repo, commenter, e, cfg, log)
	}

	return false, nil
}

func (bot *robot) isOwnerOfSig(
	org, repo, commenter string,
	e *gitlab.MergeCommentEvent,
	cfg *botConfig,
	log *logrus.Entry,
) (bool, error) {
	changes, err := bot.cli.GetMergeRequestChanges(e.ProjectID, e.MergeRequest.IID)
	if err != nil || len(changes) == 0 {
		return false, err
	}

	paths := sets.NewString()
	for _, file := range changes {
		if file == "" {
			continue
		}
		if !cfg.regSigDir.MatchString(file) || strings.Count(file, "/") > 2 {
			fmt.Println("return")
			return false, nil
		}

		paths.Insert(filepath.Dir(file))
	}

	fmt.Println("paths == ", paths)

	// get directory tree
	oPath, sPath, err := bot.listDirectoryTree(e.ProjectID, "master", cfg.SigsDir)
	if err != nil || len(oPath) == 0 || len(sPath) == 0 {
		return false, nil
	}

	for _, o := range oPath {
		p := filepath.Dir(o)
		if !paths.Has(p) {
			continue
		}

		fmt.Println("geeeeeeettttttt in")
		oFile, err := bot.cli.GetPathContent(e.ProjectID, o, "master")
		if err != nil || oFile == nil {
			return false, nil
		}

		if o := decodeOwnerFile(oFile.Content, log); !o.Has(commenter) {
			return false, nil
		}

		paths.Delete(p)

		if len(paths) == 0 {
			return true, nil
		}
	}

	for _, s := range sPath {
		p := filepath.Dir(s)
		if !paths.Has(p) {
			continue
		}

		sFile, err := bot.cli.GetPathContent(e.ProjectID, s, "master")
		if err != nil || sFile == nil {
			return false, nil
		}

		if o := decodeSigInfoFile(sFile.Content, log); !o.Has(commenter) {
			return false, nil
		}

		paths.Delete(p)

		if len(paths) == 0 {
			return true, nil
		}
	}

	//ownerFiles, err := bot.getFiles(org, repo, e.MergeRequest.TargetBranch, ownerFile, log)
	//if err != nil {
	//	return false, err
	//}
	//
	//sigInfoFiles, err := bot.getFiles(org, repo, e.MergeRequest.TargetBranch, sigInfoFile, log)
	//if err != nil {
	//	return false, err
	//}
	//
	//for _, v := range ownerFiles.Files {
	//	p := v.Path.Dir()
	//	if !paths.Has(p) {
	//		continue
	//	}
	//
	//	if o := decodeOwnerFile(v.Content, log); !o.Has(commenter) {
	//		return false, nil
	//	}
	//
	//	paths.Delete(p)
	//
	//	if len(paths) == 0 {
	//		return true, nil
	//	}
	//}
	//
	//for _, v := range sigInfoFiles.Files {
	//	p := v.Path.Dir()
	//	if !paths.Has(p) {
	//		continue
	//	}
	//
	//	if o := decodeSigInfoFile(v.Content, log); !o.Has(commenter) {
	//		return false, nil
	//	}
	//
	//	paths.Delete(p)
	//
	//	if len(paths) == 0 {
	//		return true, nil
	//	}
	//}

	return false, nil
}

func (bot *robot) listDirectoryTree(pid int, branch, dirPath string) ([]string, []string, error) {
	recursive := true
	ownerFilePath := make([]string, 0)
	sigInfoFilePath := make([]string, 0)
	opt := gitlab.ListTreeOptions{Path: &dirPath, Ref: &branch, Recursive: &recursive}
	trees, err := bot.cli.GetDirectoryTree(pid, opt)
	if err != nil {
		return nil, nil, err
	}

	for _, t := range trees {
		if strings.Count(t.Path, "/") == 2 && strings.Contains(t.Path, ownerFile) {
			ownerFilePath = append(ownerFilePath, t.Path)
		}

		if strings.Count(t.Path, "/") == 2 && strings.Contains(t.Path, sigInfoFile) {
			sigInfoFilePath = append(sigInfoFilePath, t.Path)
		}
	}

	return ownerFilePath, sigInfoFilePath, nil
}

func (bot *robot) getFiles(org, repo, branch, fileName string, log *logrus.Entry) (models.FilesInfo, error) {
	files, err := bot.cacheCli.GetFiles(
		models.Branch{
			Platform: "gitlab",
			Org:      org,
			Repo:     repo,
			Branch:   branch,
		},
		fileName, false,
	)
	if err != nil {
		return models.FilesInfo{}, err
	}

	if len(files.Files) == 0 {
		log.WithFields(
			logrus.Fields{
				"org":    org,
				"repo":   repo,
				"branch": branch,
			},
		).Infof("there is not %s file stored in cache.", fileName)
	}

	return files, nil
}

func decodeSigInfoFile(content string, log *logrus.Entry) sets.String {
	owners := sets.NewString()

	c, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		log.WithError(err).Error("decode file")

		return owners
	}

	var m SigInfos

	if err = yaml.Unmarshal(c, &m); err != nil {
		log.WithError(err).Error("code yaml file")

		return owners
	}

	for _, v := range m.Maintainers {
		owners.Insert(strings.ToLower(v.GiteeID))
	}

	fmt.Println("owners ******************** ", owners)

	return owners
}

func decodeOwnerFile(content string, log *logrus.Entry) sets.String {
	owners := sets.NewString()

	c, err := base64.StdEncoding.DecodeString(content)
	if err != nil {
		log.WithError(err).Error("decode file")

		return owners
	}

	var m struct {
		Maintainers []string `yaml:"maintainers"`
		Committers  []string `yaml:"committers"`
	}

	if err = yaml.Unmarshal(c, &m); err != nil {
		log.WithError(err).Error("code yaml file")

		return owners
	}

	for _, v := range m.Maintainers {
		owners.Insert(strings.ToLower(v))
	}

	for _, v := range m.Committers {
		owners.Insert(strings.ToLower(v))
	}

	fmt.Println("owners ************** ", owners)
	return owners
}
