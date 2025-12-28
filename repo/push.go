package repo

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/goeoeo/gitx/model"
	"github.com/goeoeo/gitx/util"
	"github.com/sirupsen/logrus"
	"github.com/xanzy/go-gitlab"
)

var (
	ErrStop = errors.New("don`t continue")
)

type RepoPatch struct {
	Repo              *Repo
	Patch             *Patch
	jm                *model.JiraMgr
	config            *Config
	ignoreLocalCommit bool
}

func NewRepoPatch(repo *Repo, config *Config) *RepoPatch {
	return &RepoPatch{
		Repo:   repo,
		Patch:  config.Patch,
		config: config,
	}
}

// IgnoreLocalCommit 忽略本地记录，cherry-pick所有commit"
func (rp *RepoPatch) IgnoreLocalCommit(f bool) *RepoPatch {
	rp.ignoreLocalCommit = f
	return rp
}

func (rp *RepoPatch) Push() (results []*RepoPushResult, err error) {
	var (
		rpr *RepoPushResult
	)

	if rp.jm, err = model.NewJiraMgr(); err != nil {
		return nil, err
	}

	jira := rp.jm.GetOrCreate(rp.Repo.Name, rp.Patch.JiraId, rp.Patch.CommitType, rp.Patch.CommitMsg)
	jira.AddTargetBranch(rp.Patch.GetPlanTgtBranchList())

	for _, tgtBranch := range rp.Patch.GetTgtBranchs() {
		pRepo := NewRepoPush(rp.Repo, rp.config, tgtBranch, jira, rp.ignoreLocalCommit)
		if err = pRepo.GitRepo.LsRemote(); err != nil {
			logrus.Debugf("git remote connection exception, please check; repo: %s \n", rp.Repo.Path)
			return nil, err
		}

		if rpr, err = pRepo.push(); err != nil {
			if errors.Is(err, ErrStop) {
				logrus.Debugf("user stop")
				return nil, nil
			}

			logrus.Debugf("git repo push faild: repo: %s, target branch [%s], err: %v \n",
				rp.Repo.Path, tgtBranch, err)
			return
		}

		if rpr.TargetBranch != "" {
			results = append(results, rpr)
		}

	}

	//推送完成后会到自己的dev分支
	if err := NewGitRepo(rp.Repo.Path, rp.Repo.Url).SwitchBranch(rp.Patch.DevBranch); err != nil {
		return nil, err
	}

	if len(jira.BranchList) == 0 {
		return nil, fmt.Errorf("未提取到提交信息，项目目录搞错了？")
	}
	//保存
	if err = rp.jm.Save(); err != nil {
		return nil, err
	}

	return
}

type (
	RepoPush struct {
		GitRepo           *GitRepo
		RepoPushPatch     *RepoPushPatch
		jr                *model.Jira
		config            *Config
		repo              *Repo
		ignoreLocalCommit bool
	}
	RepoPushPatch struct {
		DevBranch string
		TgtBranch string
		JiraId    string
		JiraDesc  string
	}

	RepoPushResult struct {
		MergeRes     string
		Project      string
		DevBranch    string
		TargetBranch string
		MergeUrl     string
		NewBranch    string
		OutCommits   []*model.CommitInfo
	}
)

func NewRepoPush(r *Repo, config *Config, tgtBranch string, jr *model.Jira, ignoreLocalCommit bool) *RepoPush {
	repoPushPatch := &RepoPushPatch{
		DevBranch: config.Patch.DevBranch,
		TgtBranch: tgtBranch,
		JiraId:    config.Patch.JiraId,
		JiraDesc:  config.Patch.JiraDesc,
	}
	return &RepoPush{
		GitRepo:           NewGitRepo(r.Path, r.Url),
		RepoPushPatch:     repoPushPatch,
		jr:                jr,
		config:            config,
		repo:              r,
		ignoreLocalCommit: ignoreLocalCommit,
	}
}

// https://cwiki.yunify.com/pages/viewpage.action?pageId=132639361
// 1. 基于 目标分支 创建一个新分支 JiraId_JiraDesc_TgtBranch
// 2. 从 DevBranch 找出 jira 的 commit。 所以要求 commit message 用 jira id 开头
// 3. 用 git cherry-pick 把 commit 提交到 JiraId_JiraDesc_TgtBranch
// 4. 生成 JiraId_JiraDesc_TgtBranch 到 TgtBranch 的 merge request url.
func (r *RepoPush) push() (result *RepoPushResult, err error) {
	var (
		mergeReq               string
		newBranch              string
		tmpCommits, cis        []*model.CommitInfo
		ret                    bool
		mrInfo                 *model.MrInfo
		mergeRes               string
		newBranchRebaseSuccess bool
	)

	logrus.Debugf("begin push repo [%s] branch [%s] ... \n", r.GitRepo.Path, r.RepoPushPatch.TgtBranch)

	tgtBranch := r.RepoPushPatch.TgtBranch
	devBranch := r.RepoPushPatch.DevBranch
	jiraId := r.RepoPushPatch.JiraId
	repoPath := r.GitRepo.Path
	result = &RepoPushResult{
		DevBranch: devBranch,
	}

	if devBranch == tgtBranch {
		return nil, fmt.Errorf("当前分支与目标分支不能是一个%s", devBranch)
	}

	newBranch = r.newBranchName(jiraId, r.RepoPushPatch.JiraDesc, tgtBranch)

	if err = r.GitRepo.SwitchBranch(tgtBranch); err != nil {
		logrus.Debugf("switch git branch faild: repo: %s, branch [%s], err: %v \n", r.GitRepo.Path, tgtBranch, err)
		//切换目标分支失败，从远端拉取
		if err = r.GitRepo.NewBranchFromRemote(tgtBranch); err != nil {
			return nil, err
		}

	} else {
		//拉取远端分支到本地
		if err = r.GitRepo.Pull(); err != nil {
			logrus.Debugf("git pull faild: repo: %s, branch [%s], err: %v \n", r.GitRepo.Path, tgtBranch, err)
			//切回开发分支
			if err = r.GitRepo.SwitchBranch(devBranch); err != nil {
				return
			}

			//远端分支和本地不一致，删除本地分支
			if err = r.GitRepo.DelLocalBranch(tgtBranch); err != nil {
				return
			}

			//切换目标分支失败，从远端拉取
			if err = r.GitRepo.NewBranchFromRemote(tgtBranch); err != nil {
				return nil, err
			}
		}

	}

	// 但由于是目标分支，删除是否合适或风险大小，再考虑一下吧。
	if ret, err = r.GitRepo.HasBranch(newBranch); err != nil {
		logrus.Debugf("git has branch faild: repo: %s, branch [%s], err: %v \n", r.GitRepo.Path, newBranch, err)
		return
	}

	if ret && newBranch != devBranch {
		//尝试对远端分支进行rebase,如果能变基成功，说明是可合并的
		if err = r.GitRepo.SwitchBranch(newBranch); err != nil {
			return
		}

		if err = r.GitRepo.Rebase(tgtBranch); err != nil {
			//变基失败，取消
			if err = r.GitRepo.RebaseAbort(); err != nil {
				return
			}

			if err = r.GitRepo.SwitchBranch(tgtBranch); err != nil {
				return
			}

			//rebase失败，删除本地临时分支，让研发人员处理冲突
			if err = r.GitRepo.DelLocalBranch(newBranch); err != nil {
				logrus.Debugf("git del local branch faild: repo: %s, branch [%s], err: %v \n", r.GitRepo.Path, newBranch, err)
				return
			}
		} else {
			newBranchRebaseSuccess = true
		}

		if err = r.GitRepo.SwitchBranch(tgtBranch); err != nil {
			return
		}

	}

	if !newBranchRebaseSuccess {
		if err = r.GitRepo.NewBranch(newBranch); err != nil {
			logrus.Debugf("git create branch faild: repo: %s, branch [%s], err: %v \n", r.GitRepo.Path, newBranch, err)
			return
		}
	}

	if err = r.GitRepo.SwitchBranch(devBranch); err != nil {
		logrus.Debugf("switch git branch faild: repo: %s, branch [%s], err: %v \n", r.GitRepo.Path, devBranch, err)
		return
	}

	if tmpCommits, err = r.GitRepo.GetCommitInfo(r.jr.GetCherryPickMsg()); err != nil {
		logrus.Debugf("git jira %s commits faild: repo: %s, branch [%s], err: %v \n", jiraId, r.GitRepo.Path, devBranch, err)
		return
	}

	for _, commit := range tmpCommits {
		if !r.ignoreLocalCommit && r.jr.BranchContainCommit(tgtBranch, commit.CommitId) {
			continue
		}
		cis = append(cis, commit)
	}

	if len(cis) == 0 {
		return nil, fmt.Errorf("未提取到提交信息，项目目录搞错了？当前目录:%s,当前分支:%s", r.repo.Path, r.config.Patch.DevBranch)
	}

	var rows [][]string
	for _, v := range cis {
		commitID := ""
		if len(v.CommitId) > 10 {
			commitID = v.CommitId[0:10]
		}

		rows = append(rows, []string{r.repo.Name, jiraId, strings.Replace(v.Desc, " ", "", -1),
			fmt.Sprintf("%s=>%s", devBranch, tgtBranch), newBranch, commitID, v.CreateTime.Format("2006-01-02 15:04:05")})
	}

	util.PrintTable(rows, []string{"项目", "JiraID", "描述", "分支", "临时分支", "commit", "时间"})

	// 交互式 确认 commit, todo: add/remove/re commit
	showCommit := func() error {
	loop:
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("请确认 commit 是否正确，程序将会按照倒叙依次 cherry-pick, 输入 y 继续， 输入 n 程序退出。")
		char, _, err := reader.ReadRune()
		if err != nil {
			log.Println(err)
			return err
		}
		switch char {
		case 'y':
			return nil
		case 'n':
			return ErrStop
		default:
			goto loop
		}
	}

	if err = showCommit(); err != nil {
		return
	}

	if err = r.GitRepo.SwitchBranch(newBranch); err != nil {
		logrus.Debugf("git create branch faild: repo: %s, branch [%s], err: %v \n",
			r.GitRepo.Path, newBranch, err)
		return
	}

	checkCommit := func(commit *model.CommitInfo) error {
	checkLoop:
		//  等待用户手动处理冲突
		reader1 := bufio.NewReader(os.Stdin)
		fmt.Printf("\n cherry-pick 冲突: %s, commitID:%s ,时间:%s\n", commit.Desc, commit.CommitId[0:10], commit.CreateTime.Format("2006-01-02 15:04:05"))
		fmt.Println("请手动处理冲突, 处理完成后 输入 y 继续; 输入 s 会自动执行 cherry-pick --skip 并继续后续; 输入 n 程序退出")
		char, _, err := reader1.ReadRune()
		if err != nil {
			log.Println(err)
			return nil
		}
		switch char {
		case 'y':
			result.AddCommits(commit)
			return nil
		case 'n':
			return ErrStop
		case 's':
			result.AddCommits(commit)
			return r.GitRepo.CherryPickSkip()
		default:
			goto checkLoop
		}
	}

	for i := len(cis) - 1; i >= 0; i-- {
		commit := cis[i]
		skip := false
		if skip, err = r.GitRepo.CherryPick(commit.CommitId); err == nil {
			if !skip {
				result.AddCommits(commit)
			} else {
				fmt.Printf("目标分支已包含该commit[%s,%s]\n", tgtBranch, commit.CommitId[0:10])
				commit.TargetExists = true
				result.AddCommits(commit)
			}
			continue
		}
		logrus.Debugf("git cherry-pick commit [%s] faild: repo: %s, branch [%s], err: %v \n",
			commit.CommitId, r.GitRepo.Path, tgtBranch, err)

		if err = checkCommit(commit); err != nil {
			return
		}
	}

	// 先删远程，再 push， 简化流程，避免冲突造成的额外工作。
	//_ = r.GitRepo.DelRemoteBranch(newBranch)
	if err = r.GitRepo.Push(newBranch, tgtBranch); err != nil {
		logrus.Debugf("git push faild: repo: %s, branch [%s], err: %v \n",
			r.GitRepo.Path, newBranch, err)
		return
	}

	if result.NewCommitsLen() == 0 {
		return
	}

	//生成JiraBranch
	jb := &model.JiraBranch{
		BranchName:   newBranch,
		DevBranch:    devBranch,
		TargetBranch: tgtBranch,
		Merged:       false,
		Commits:      result.OutCommits,
	}

	mergeReq = r.GitRepo.NewMergeReq(newBranch, tgtBranch)

	//自动创建mr
	if r.repo.CreateMr {
		//util.PrintJson(jb)
		if mrInfo, err = r.GitRepo.CreateMergeRequest(jb.Desc(true), jb.BranchName, jb.TargetBranch); err != nil {
			return
		}

		jb.MergeRequests = append(jb.MergeRequests, mrInfo)
		result.MergeUrl = mrInfo.WebUrl
		mergeReq = mrInfo.WebUrl
		//自动合并
		if util.ContainString(r.repo.AutoMergeBranchList, jb.TargetBranch) {
			if mergeRes, err = r.GitRepo.AcceptMergeRequest(mrInfo.MrId); err != nil {
				return
			}
			result.MergeRes = mergeRes

			//尝试执行hook
			_, ok := r.repo.AutoMergeBranchHook[r.RepoPushPatch.TgtBranch]
			if mergeRes != MergeResFail && r.config.Patch.AutoMergeHook && ok {
				if r.waitMrMerged(mrInfo.MrId) {
					r.AutoMergeBranchHook()
					result.MergeRes = MergeResOk
				}
			}
		}

	}

	r.jr.AttachBranch(tgtBranch).Append(jb)

	logrus.Debugf("git push ok: jiraId: %s repo: %s, branch [%s] to branch [%s]; merge url:\n %v \n",
		jiraId, repoPath, newBranch, tgtBranch, mergeReq)

	result.NewBranch = newBranch
	result.MergeUrl = mergeReq
	result.TargetBranch = tgtBranch
	result.Project = r.jr.Project
	return
}

// waitMrMerged 登台MR合并
func (r *RepoPush) waitMrMerged(mrId int) (merged bool) {
	var (
		err error
		mr  *gitlab.MergeRequest
	)
	defer func() {
		if err != nil {
			logrus.Infof("waitMrMerged err:%s", err)
		}
	}()
	num := 0
	for {
		// 等待10分钟
		if num > 120 {
			break
		}

		if mr, err = r.GitRepo.GetMergeRequest(mrId); err != nil {
			break
		}

		if mr.State == "merged" {
			merged = true
			break
		}

		time.Sleep(5 * time.Second)
		num++
	}

	return
}

func (r *RepoPush) AutoMergeBranchHook() {
	cmdStrArr, ok := r.repo.AutoMergeBranchHook[r.RepoPushPatch.TgtBranch]
	if !ok {
		return
	}

	for _, cmdStr := range cmdStrArr {
		// 使用Command函数解析字符串为命令
		cmd := exec.Command("sh", "-c", cmdStr)

		// 运行命令，并获取输出
		output, err := cmd.CombinedOutput()
		if err != nil {
			logrus.Infof("AutoMergeBranchHook err:%s", err)
			return
		}

		fmt.Printf("执行hook:%s\n 输出结果:\n%s\n", cmdStr, string(output))
	}

}

func (r *RepoPush) newBranchName(jiraID, jiraDesc, tgtBranch string) string {
	m := map[string]string{
		"jiraID":    jiraID,
		"jiraDesc":  jiraDesc,
		"tgtBranch": tgtBranch,
	}

	s := r.config.Patch.TmpBranchFmt
	for k, v := range m {
		s = strings.Replace(s, fmt.Sprintf("{%s}", k), v, 1)
	}

	return s
}

func (rpr *RepoPushResult) AddCommits(commit *model.CommitInfo) {
	rpr.OutCommits = append(rpr.OutCommits, commit)
}

func (rpr *RepoPushResult) NewCommitsLen() int {
	num := 0
	for _, v := range rpr.OutCommits {
		if !v.TargetExists {
			num++
		}
	}
	return num
}
