package controller

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/goeoeo/gitx/model"
	"github.com/goeoeo/gitx/repo"
	"github.com/goeoeo/gitx/util"
	"github.com/sirupsen/logrus"
)

type JiraController struct {
	config        *repo.Config
	jm            *model.JiraMgr
	projectBranch map[string][]string
}

func NewJiraController(config *repo.Config) (jc *JiraController, err error) {
	jc = &JiraController{
		config:        config,
		projectBranch: make(map[string][]string),
	}

	//载入jira数据
	jc.jm, err = model.NewJiraMgr()

	return
}

// Clear 清理远程分支没有MR且超过一周的分支
// 配合定时任务，保持本地项目和远程项目的分支简洁性
func (jc *JiraController) Clear() (err error) {
	logrus.Infof("开始清理分支，当前jira数据库中的项目数量:%d\n", len(jc.jm.JiraList))

	// 载入jira数据
	for _, j := range jc.jm.JiraList {
		logrus.Infof("处理项目:%s\n", j.Project)
		for _, jb := range j.BranchList {
			if err = jc.delBranch(j, jb); err != nil {
				logrus.Errorf("删除分支错误:%s\n", err)
			}
		}
	}

	// 持久化
	err = jc.jm.Save()
	return
}

// Add 添加Jira
func (jc *JiraController) Add(project, jiraID string, targetBranch []string) (err error) {
	repoCfg := jc.config.Repo[project]
	if repoCfg == nil {
		return fmt.Errorf("项目仓库信息缺失:%s", project)
	}

	if len(targetBranch) == 0 {
		return fmt.Errorf("目标分支不能为空")
	}

	//翻译分支名
	targetBranch = jc.config.TransBranch(targetBranch)

	return jc.jm.AddJira(project, jiraID, targetBranch)
}

// Del 删除Jira
func (jc *JiraController) Del(project, jiraID string) (err error) {
	return jc.jm.DelJira(project, jiraID)
}

func (jc *JiraController) Detach(project, jiraID, branch string) error {
	return jc.jm.Detach(project, jiraID, branch)
}

func (jc *JiraController) delBranch(j *model.Jira, jb *model.JiraBranch) (err error) {
	var (
		branchTime time.Time
		repoPath   string
		repoUrl    string
	)

	// 如果已经标记为已合入，则直接跳过
	if jb.Merged {
		logrus.Infof("跳过，分支已标记为已合入:%s \n", jb.BranchName)
		return
	}

	if jb.DevBranch == "" {
		return
	}

	// 获取项目配置
	repoCfg := jc.config.GetProjectRepoUrl(j.Project)
	if repoCfg == nil {
		logrus.Debugf("项目仓库信息缺失:%s，跳过", j.Project)
		return nil
	}
	repoPath = repoCfg.Path
	repoUrl = repoCfg.Url

	// 创建GitRepo实例
	git := repo.NewGitRepo(repoPath, repoUrl)

	// 检查分支是否超过一周
	branchTime, err = git.GetBranchCreateTime(jb.BranchName)
	if err != nil {
		logrus.Debugf("获取分支时间错误:%s\n", err)
		// 即使获取时间失败，也标记为已合入，避免重复处理
		jb.Merged = true
		return nil
	}

	// 检查是否超过一周
	if time.Since(branchTime) <= 7*24*time.Hour {
		logrus.Infof("跳过，分支未超过一周:%s \n", jb.BranchName)
		return
	}

	logrus.Infof("分支没有MR且超过一周，准备删除:%s \n", jb.BranchName)

	// 删除远程分支 - 即使失败也继续，因为分支可能已经不存在
	if err := git.DelRemoteBranch(jb.BranchName); err != nil {
		logrus.Debugf("删除远程分支错误:%s\n", err)
		// 分支不存在也视为删除成功
		if strings.Contains(err.Error(), "远程引用不存在") || strings.Contains(err.Error(), "remote ref does not exist") {
			logrus.Infof("远程分支不存在，视为删除成功:%s\n", jb.BranchName)
		}
	}

	// 删除本地分支 - 即使失败也继续，因为分支可能已经不存在
	if err := git.DelLocalBranch(jb.BranchName); err != nil {
		logrus.Debugf("删除本地分支错误:%s\n", err)
		// 分支不存在也视为删除成功
		if strings.Contains(err.Error(), "错误：无法删除") || strings.Contains(err.Error(), "error: Cannot delete") {
			logrus.Infof("本地分支不存在，视为删除成功:%s\n", jb.BranchName)
		}
	}

	// 标记为已合入，下次跳过
	jb.Merged = true
	logrus.Infof("分支已标记为已合入:%s\n", jb.BranchName)

	return
}

func (jc *JiraController) branchExists(j *model.Jira, jb *model.JiraBranch) (exists bool, err error) {
	var (
		branchList []string
	)

	if jb.BranchName == "" {
		return false, nil
	}

	if _, ok := jc.projectBranch[j.Project]; !ok {
		repoCfg := jc.config.GetProjectRepoUrl(j.Project)
		if repoCfg == nil {
			logrus.Debugf("项目仓库信息缺失:%s，跳过分支检查", j.Project)
			return false, nil
		}

		git := repo.NewGitRepo(repoCfg.Path, repoCfg.Url)
		if branchList, err = git.GetBranchs(); err != nil {
			logrus.Debugf("获取本地分支错误:%s\n", err)
			branchList = []string{}
		}
		// 获取远程分支
		var remoteBranchs []string
		if remoteBranchs, err = git.GetRemoteBranchs(); err != nil {
			logrus.Debugf("获取远程分支错误:%s\n", err)
			remoteBranchs = []string{}
		}
		// 合并本地和远程分支
		branchList = append(branchList, remoteBranchs...)
		jc.projectBranch[j.Project] = branchList
	}

	branchList = jc.projectBranch[j.Project]
	for _, v := range branchList {
		// 检查分支名是否匹配
		if v == jb.BranchName {
			return true, nil
		}
	}

	return false, nil

}

func (jc *JiraController) CheckBranchMerged(project, jiraId string) (err error) {
	for _, j := range jc.jm.JiraList {

		if project != "" && j.Project != project {
			continue
		}

		if jiraId != "" && j.JiraID != jiraId {
			continue
		}

		for _, jb := range j.BranchList {
			merged, err := jc.checkBranchMerged(j, jb)
			if err != nil {
				return err
			}

			fmt.Printf("project:%s,jiradID:%s,Branch:%s,Merged:%t\n", j.Project, j.JiraID, jb.BranchName, merged)

		}
	}

	return

}

// checkBranchMerged 检查分支对应的JIRA是否已合并
func (jc *JiraController) checkBranchMerged(j *model.Jira, jb *model.JiraBranch) (merged bool, err error) {
	var (
		commits []*model.CommitInfo
	)

	if jb.BranchName == "" || jb.Merged {
		return
	}
	repoCfg := jc.config.Repo[j.Project]
	if repoCfg == nil || repoCfg.Url == "" {
		return false, fmt.Errorf("项目仓库信息缺失:%s", j.Project)
	}
	logrus.Debugf("satrt checkBranchMerged:%s", j.Project)
	git := repo.NewGitRepo(repoCfg.Path, repoCfg.Url)
	//check 目标分支
	if err = git.SwitchBranch(jb.TargetBranch); err != nil {
		return
	}
	defer func() {
		_ = git.ResetBranch()
	}()

	//拉取最新代码
	if err = git.Pull(); err != nil {
		if err = git.ResetBranch(); err != nil {
			return
		}

		if err = git.DelLocalBranch(jb.TargetBranch); err != nil {
			return
		}

		if err = git.NewBranchFromRemote(jb.TargetBranch); err != nil {
			return
		}

		if err = git.Pull(); err != nil {
			return
		}

	}

	if commits, err = git.GetCommitInfo(j.GetCherryPickMsg()); err != nil {
		return
	}

	lci := jb.LastCommitInfo()
	if lci == nil {
		return
	}

	logrus.Debugf(">>>>>>%s,lastcommitTime:%s", jb.TargetBranch, lci.CreateTime)
	//util.PrintJson(commits)

	after := false
	for _, c := range commits {
		if c.CreateTime.After(lci.CreateTime) {
			after = true
			break
		}
	}

	//远程分支的提交信息没有比 jb中最大的时间大，说明远程还未合入
	if !after {
		return
	}

	//已合并
	return true, nil

}

// Print 打印出那些为合并完成的Jira
func (jc *JiraController) Print(project, jiraId string) (err error) {
	var (
		rows [][]string
	)

	if err = jc.syncMergeInfo(project, jiraId); err != nil {
		return fmt.Errorf("同步merge信息错误:%v", err)
	}

	for _, jr := range jc.jm.JiraList {
		// 指定了JiraID时，忽略项目名称匹配
		if jiraId != "" {
			if jr.JiraID != jiraId {
				continue
			}
		} else if project != "" && jr.Project != project {
			continue
		}

		// 如果用户明确指定了jiraId，即使任务已完成也打印
		if jr.Complete() && jiraId == "" {
			continue
		}

		sort.Slice(jr.BranchList, func(i, j int) bool {
			if jr.BranchList[i].DevBranch != jr.BranchList[j].DevBranch {
				return jr.BranchList[i].DevBranch > jr.BranchList[j].DevBranch
			}
			return jr.BranchList[i].TargetBranch < jr.BranchList[j].TargetBranch
		})

		rows = append(rows, []string{jr.GetDesc(), "MR", "状态", "更新时间"})

		for _, jb := range jr.BranchList {
			status := "待提交"
			if jb.DevBranch != "" {
				status = "待合并"
			}

			if jb.DevBranch != "" && jb.Merged {
				status = "已合并"
			}

			rows = append(rows, []string{fmt.Sprintf("%s=>%s", jb.DevBranch, jb.TargetBranch), jb.MR(), status, jb.UpdateTime.Format("2006-01-02 15-04-05")})
		}

		l := ""
		rows = append(rows, []string{l, l, l})
	}

	if len(rows) > 0 {
		rows = rows[0 : len(rows)-1]
	}

	util.PrintTable(rows, nil)

	return
}

// syncMergeInfo 合并同步信息
func (jc *JiraController) syncMergeInfo(project, jiraId string) (err error) {
	var (
		merged   bool
		saveData bool
	)
	for _, jr := range jc.jm.JiraList {
		if project != "" && jr.Project != project {
			continue
		}

		if jiraId != "" && jiraId != jr.JiraID {
			continue
		}

		for _, jb := range jr.BranchList {
			if jb.Merged || jb.DevBranch == "" {
				continue
			}

			// 检查是否有可用的仓库信息
			repoCfg := jc.config.Repo[jr.Project]
			if repoCfg == nil || repoCfg.Url == "" || repoCfg.Path == "" {
				logrus.Debugf("项目 %s 仓库信息缺失，跳过分支合并检查", jr.Project)
				continue // 跳过此分支的合并检查
			}

			if merged, err = jc.checkBranchMerged(jr, jb); err != nil {
				logrus.Debugf("检查分支 %s 合并状态错误: %v，跳过", jb.BranchName, err)
				err = nil // 重置错误，继续处理其他分支
				continue
			}

			if merged {
				saveData = true
				jb.Merged = true
			}
		}
	}

	if !saveData {
		return
	}

	err = jc.jm.Save()
	return
}
