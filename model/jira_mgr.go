package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/goeoeo/gitx/util"
)

type JiraMgr struct {
	saveDir  string
	jsonPath string
	JiraList []*Jira
}

func NewJiraMgr() (jm *JiraMgr, err error) {
	d, _ := os.UserHomeDir()
	jm = &JiraMgr{
		saveDir:  filepath.Join(d, ".patch"),
		jsonPath: filepath.Join(d, ".patch", "jira.json"),
	}

	// 确保保存目录存在
	if err = os.MkdirAll(jm.saveDir, 0755); err != nil {
		return nil, fmt.Errorf("create save directory error: %v", err)
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

func (jm *JiraMgr) load() (err error) {
	// 检查文件是否存在
	if _, err = os.Stat(jm.jsonPath); os.IsNotExist(err) {
		// 文件不存在，初始化空列表
		jm.JiraList = []*Jira{}
		return nil
	}

	b, err := os.ReadFile(jm.jsonPath)
	if err != nil {
		return fmt.Errorf("read jira.json error: %v", err)
	}

	if len(b) == 0 {
		// 空文件，初始化空列表
		jm.JiraList = []*Jira{}
		return nil
	}

	// 解析 JSON 数据
	if err = json.Unmarshal(b, &jm.JiraList); err != nil {
		return fmt.Errorf("unmarshal jira.json error: %v", err)
	}

	return
}

func (jm *JiraMgr) Save() (err error) {
	// 序列化 JiraList 为 JSON
	b, err := json.MarshalIndent(jm.JiraList, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal jira data error: %v", err)
	}

	// 写入文件
	if err = os.WriteFile(jm.jsonPath, b, 0644); err != nil {
		return fmt.Errorf("write jira.json error: %v", err)
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
