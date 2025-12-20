package model

import (
	"encoding/json"
	"github.com/goeoeo/gitx/util"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"
)

type JiraMgr struct {
	saveDir  string
	JiraList []*Jira
}

func NewJiraMgr() (jm *JiraMgr, err error) {
	d, _ := os.UserHomeDir()
	jm = &JiraMgr{
		saveDir: filepath.Join(d, ".patch"),
	}
	if err = jm.load(); err != nil {
		return
	}

	for _, v := range jm.JiraList {
		v.Init()
	}
	return
}

func (jm *JiraMgr) AddJira(project, jiraID string, targetBranch []string) (err error) {
	j := jm.GetOrCreate(project, jiraID, CommitTypeJira, "")

	j.TargetBranch = append(j.TargetBranch, targetBranch...)
	j.TargetBranch = util.Unique(j.TargetBranch)
	j.UpdateTime = time.Now()

	return jm.Save()
}

func (jm *JiraMgr) DelJira(project, jiraID string) (err error) {
	var (
		jrl []*Jira
	)

	find := false
	for _, v := range jm.JiraList {
		if v.Project == project && v.JiraID == jiraID {
			find = true
			continue
		}

		jrl = append(jrl, v)
	}

	if !find {
		return
	}

	jm.JiraList = jrl

	return jm.Save()
}

func (jm *JiraMgr) Detach(project, jiraID, branch string) (err error) {
	var (
		targetBranchList []string
		branchList       []*JiraBranch
	)
	for _, v := range jm.JiraList {
		if v.Project == project && v.JiraID == jiraID {

			for _, b := range v.TargetBranch {
				if b != branch {
					targetBranchList = append(targetBranchList, b)
				}
			}

			v.TargetBranch = targetBranchList
			for _, b := range v.BranchList {
				if b.TargetBranch != branch {
					branchList = append(branchList, b)
				}
			}

			v.BranchList = branchList

		}

	}
	return jm.Save()

}

func (jm *JiraMgr) savePath() string {
	return filepath.Join(jm.saveDir, "jira.json")
}

func (jm *JiraMgr) load() (err error) {
	var (
		b []byte
	)

	// 文件不存在，创建
	if _, err = os.Stat(jm.savePath()); err != nil {
		if err = ioutil.WriteFile(jm.savePath(), []byte("[]"), fs.ModePerm); err != nil {
			return
		}
	}

	if b, err = ioutil.ReadFile(jm.savePath()); err != nil {
		return
	}

	if len(b) > 0 {
		if err = json.Unmarshal(b, &jm.JiraList); err != nil {
			return
		}
	}

	return
}

func (jm *JiraMgr) Save() (err error) {
	var (
		b []byte
	)

	if b, err = json.MarshalIndent(jm.JiraList, "", "  "); err != nil {
		return
	}

	if len(b) == 0 || len(jm.JiraList) == 0 {
		return
	}

	if err = ioutil.WriteFile(jm.savePath(), b, fs.ModePerm); err != nil {
		return
	}

	return
}

func (jm *JiraMgr) get(project, jiraID string) *Jira {
	for _, v := range jm.JiraList {
		if v.Project == project && v.JiraID == jiraID {
			return v
		}
	}
	return nil
}

func (jm *JiraMgr) GetOrCreate(project, jiraID, commitType, commitMsg string) *Jira {
	j := jm.get(project, jiraID)
	if j != nil {
		return j
	}

	j = &Jira{
		Project:       project,
		JiraID:        jiraID,
		CommitType:    commitType,
		CommitMessage: commitMsg,
		CreateTime:    time.Now(),
		UpdateTime:    time.Now(),
	}
	jm.JiraList = append(jm.JiraList, j)
	return j
}
