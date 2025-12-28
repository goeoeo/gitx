package repo

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/goeoeo/gitx/model"
	"github.com/sirupsen/logrus"
	"github.com/xanzy/go-gitlab"
)

const (
	MergeResOk           = "ok"
	MergeResWaitPipeline = "wait-pipeline"
	MergeResFail         = "fail"
)

type GitRepo struct {
	Path          string // 本地路径
	Url           string // https url
	gitlabConfig  *GitLabConfig
	currentBranch string //记录工作区操作之前的分支
}

func NewGitRepo(path string, url string) *GitRepo {
	return &GitRepo{
		Path:          path,
		Url:           url,
		gitlabConfig:  GetConfig().GetGitLabConfig(url),
		currentBranch: AutoBranch(path),
	}
}

func (g *GitRepo) GetBranchs() ([]string, error) {
	// 返回的第一个元素是当前分支
	cmdRet, err := ExecCmd(g.Path, "git", "branch")
	if err != nil {
		logrus.Debugf("get git branchs faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return nil, err
	}
	lines := strings.Split(cmdRet.Out, "\n")
	var branchs []string
	for _, line := range lines {
		branchLine := strings.TrimSpace(line)
		if len(branchLine) == 0 {
			continue
		}
		if strings.HasPrefix(branchLine, "*") {
			lineSplit := strings.Split(branchLine, " ")
			branchs = append([]string{lineSplit[1]}, branchs...)
		} else {
			branchs = append(branchs, branchLine)
		}
	}
	return branchs, nil
}

// GetBranchCreateTime 获取分支创建时间
func (g *GitRepo) GetBranchCreateTime(branch string) (time.Time, error) {
	cmdRet, err := ExecCmd(g.Path, "git", "log", "--pretty=format:%ai", "--reverse", branch, "--")
	if err != nil {
		logrus.Debugf("get branch create time faild: branch: %s, out: %s, err: %s \n", branch, cmdRet.Out, cmdRet.ErrStr)
		return time.Time{}, err
	}
	lines := strings.Split(cmdRet.Out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			// 格式：2023-01-01 12:00:00 +0800
			return time.Parse("2006-01-02 15:04:05 -0700", line)
		}
	}
	return time.Time{}, fmt.Errorf("no commit found for branch: %s", branch)
}

// GetRemoteBranchs 获取远程分支
func (g *GitRepo) GetRemoteBranchs() ([]string, error) {
	cmdRet, err := ExecCmd(g.Path, "git", "branch", "-r")
	if err != nil {
		logrus.Debugf("get remote branchs faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return nil, err
	}
	lines := strings.Split(cmdRet.Out, "\n")
	var branchs []string
	for _, line := range lines {
		branchLine := strings.TrimSpace(line)
		if len(branchLine) == 0 {
			continue
		}
		// 移除 origin/ 前缀
		branch := strings.TrimPrefix(branchLine, "origin/")
		branchs = append(branchs, branch)
	}
	return branchs, nil
}

func (g *GitRepo) GetBranch() (string, error) {
	cmdRet, err := ExecCmd(g.Path, "git", "branch")
	if err != nil {
		logrus.Debugf("get git branch faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return "", err
	}
	lines := strings.Split(cmdRet.Out, "\n")
	b := ""
	for _, line := range lines {
		if !strings.HasPrefix(line, "*") {
			continue
		}
		lineSplit := strings.Split(line, " ")
		b = lineSplit[1]
		break
	}
	log.Println("local branch is:", b)
	return b, nil
}

func (g *GitRepo) IsBranch(branch string) (bool, error) {
	if len(branch) == 0 {
		logrus.Debugf("illegal parameter: branch: %s \n", branch)
		return false, errors.New("illegal params: branch")
	}
	b, err := g.GetBranch()
	if err != nil {
		log.Println("get git branch faild:", err)
	}
	if b != branch {
		return false, nil
	}
	return true, nil
}

func (g *GitRepo) HasBranch(branch string) (bool, error) {
	if len(branch) == 0 {
		logrus.Debugf("illegal parameter: branch: %s \n", branch)
		return false, errors.New("illegal params: branch")
	}
	branchs, err := g.GetBranchs()
	if err != nil {
		log.Println("get git branchs faild:", err)
	}
	for _, v := range branchs {
		if branch == v {
			return true, nil
		}
	}
	return false, nil
}

func (g *GitRepo) SwitchBranch(branch string) error {
	cmdRet, err := ExecCmd(g.Path, "git", "checkout", branch)
	if err != nil {
		logrus.Debugf("switch git branch faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	return nil
}

// ResetBranch 回到操作前的分支
func (g *GitRepo) ResetBranch() error {
	return g.SwitchBranch(g.currentBranch)
}

func (g *GitRepo) NewBranch(branch string) error {
	cmdRet, err := ExecCmd(g.Path, "git", "checkout", "-b", branch)
	if err != nil {
		logrus.Debugf("create new branch faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	return nil
}

func (g *GitRepo) NewBranchFromRemote(branch string) error {
	cmdRet, err := ExecCmd(g.Path, "git", "checkout", "-b", branch, "origin/"+branch)
	if err != nil {
		logrus.Debugf("create new branch from remote faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	return nil
}

func (g *GitRepo) Rebase(branch string) error {
	cmdRet, err := ExecCmd(g.Path, "git", "rebase", branch)
	if err != nil {
		logrus.Debugf("rebase branch faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	return nil
}

func (g *GitRepo) RebaseAbort() error {
	cmdRet, err := ExecCmd(g.Path, "git", "rebase", "--abort")
	if err != nil {
		logrus.Debugf("rebase  abort  faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	return nil
}

func (g *GitRepo) DelLocalBranch(branch string) error {
	if len(branch) == 0 || branch == "master" {
		logrus.Debugf("illegal parameter: branch: %s \n", branch)
		return errors.New("illegal params: branch")
	}
	cmdRet, err := ExecCmd(g.Path, "git", "branch", "-D", branch)
	if err != nil {
		logrus.Debugf("delete local branch faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	return nil
}

func (g *GitRepo) DelRemoteBranch(branch string) error {
	// 不可删除远程的 master, qa, staging, staging_iaas, QCE_* 的分支。
	if len(branch) == 0 || branch == "master" || branch == "qa" || branch == "staging" || branch == "staging_iaas" ||
		strings.HasPrefix(branch, "QCE_") {
		logrus.Debugf("illegal parameter: branch: %s \n", branch)
		return errors.New("illegal params: branch")
	}

	// 删除远程分支前需要检查远程是否有该分支相关的mr没有合并
	if g.gitlabConfig != nil && g.gitlabConfig.Token != "" {
		gitClient, err := gitlab.NewClient(g.gitlabConfig.Token, gitlab.WithBaseURL(g.gitlabConfig.BaseUrl))
		if err == nil {
			// 查询以当前分支为源分支的未合并MR
			openedMRs, _, err := gitClient.MergeRequests.ListProjectMergeRequests(g.getPid(), &gitlab.ListProjectMergeRequestsOptions{
				State:        gitlab.String("opened"),
				SourceBranch: gitlab.String(branch),
			})
			if err == nil && len(openedMRs) > 0 {
				logrus.Warnf("跳过删除分支 %s，存在未合并的MR: %d 个\n", branch, len(openedMRs))
				for _, mr := range openedMRs {
					logrus.Warnf("MR标题: %s, URL: %s\n", mr.Title, mr.WebURL)
				}
				return fmt.Errorf("分支 %s 存在未合并的MR，无法删除", branch)
			}
		}
	}

	cmdRet, err := ExecCmd(g.Path, "git", "push", "origin", "--delete", branch)
	if err != nil {
		logrus.Debugf("delete remote branch faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	return nil
}

func (g *GitRepo) GetCommitInfo(jiraId string) (cis []*model.CommitInfo, err error) {
	var (
		commitLogs string
	)

	greps := fmt.Sprintf("--grep=%s", jiraId)
	cmdRet, err := ExecCmd(g.Path, "git", "log", "--pretty=format:%H|%s|%cd", "--no-merges", greps)
	if err != nil {
		return nil, err
	}

	commitLogs = cmdRet.Out
	lines := strings.Split(commitLogs, "\n")
	RevertCommitLogs := g.FilterRevertCommits(lines)

	for _, line := range lines {
		commitLine := strings.TrimSpace(line)
		if len(commitLine) == 0 {
			continue
		}
		lineSplit := strings.SplitN(commitLine, "|", 3)
		arr := strings.Split(lineSplit[2], " ")
		arr = arr[0 : len(arr)-1]
		timeObj, err := time.ParseInLocation(time.ANSIC, strings.Join(arr, " "), time.Local)
		if err != nil {
			return nil, err
		}

		ci := &model.CommitInfo{
			CommitId:   lineSplit[0],
			Desc:       lineSplit[1],
			CreateTime: timeObj,
		}

		if g.checkInRevertCommit(lineSplit[1], RevertCommitLogs) {
			continue
		}
		cis = append(cis, ci)
	}

	return
}

func (g *GitRepo) CherryPick(commit string) (skip bool, err error) {
	if len(commit) == 0 {
		logrus.Debugf("illegal parameter: commit: %s \n", commit)
		return false, errors.New("illegal params: commit")
	}
	cmdRet, err := ExecCmd(g.Path, "git", "cherry-pick", commit)
	if err != nil {
		logrus.Debugf("git cherry-pick faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		if strings.Contains(cmdRet.ErrStr, "--allow-empty") {
			return true, g.CherryPickSkip()
		}
		return false, err
	}
	logrus.Debugf("cherry-pick commit : %s ok!\n", commit)
	return false, nil
}

func (g *GitRepo) CherryPickSkip() error {
	cmdRet, err := ExecCmd(g.Path, "git", "cherry-pick", "--skip")
	if err != nil {
		logrus.Debugf("git cherry-pick --skip faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	logrus.Debugf("cherry-pick commit --skip ok!\n")
	return nil
}

func (g *GitRepo) Push(localBranch, srcBranch string) error {
	// git push --set-upstream origin VM-2074_VG_Migrate_qa -o 'src_branch=qa'
	cmdRet, err := ExecCmd(g.Path, "git", "push", "-f", "--set-upstream", "origin", localBranch,
		"-o", fmt.Sprintf("src_branch=%s", srcBranch))
	if err != nil {
		logrus.Debugf("git push faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	logrus.Debugf("git push : %s ok!\n", localBranch)
	return nil
}

func (g *GitRepo) Pull() error {
	// git pull
	cmdRet, err := ExecCmd(g.Path, "git", "pull")
	if err != nil {
		logrus.Debugf("git pull faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	logrus.Debugf("git pull ok!\n")
	return nil
}

func (g *GitRepo) Fetch() error {
	// git pull
	cmdRet, err := ExecCmd(g.Path, "git", "fetch")
	if err != nil {
		logrus.Debugf("git fetch faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return err
	}
	logrus.Debugf("git fetch ok!\n")
	return nil
}

func (g *GitRepo) NewMergeReq(srcBranch, targetBranch string) string {
	// merge_requests/new?merge_request%5Bsource_branch%5D=VM-2074_VG_Migrate_iaas&merge_request%5Btarget_branch%5D=staging_iaas
	srcMerge := "merge_request%5Bsource_branch%5D=" + srcBranch
	targetMerge := "merge_request%5Btarget_branch%5D=" + targetBranch
	mergeUrl := strings.TrimRight(g.Url, "/") + "/merge_requests/new?" + srcMerge + "&" + targetMerge
	return mergeUrl
}

func (g *GitRepo) LsRemote() error {
	// 判断 remote 是否可达
	timeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx1 := context.WithValue(ctx, "print", false)
	defer cancel()
	_, err := ExecCmdCtx(ctx1, g.Path, "git", "ls-remote")
	if err != nil {
		logrus.Debugf("git remote connection exception, please check; repo: %s \n", g.Path)
		return err
	}
	return nil
}

// 过滤revert Commit
func (g *GitRepo) FilterRevertCommits(commits []string) []string {
	filteredCommits := make([]string, 0)
	for _, commit := range commits {

		commitLine := strings.TrimSpace(commit)
		if len(commitLine) == 0 {
			continue
		}
		lineSplit := strings.SplitN(commitLine, " ", 2)

		if strings.HasPrefix(lineSplit[1], "Revert") {
			filteredCommits = append(filteredCommits, lineSplit[1])
		}
	}

	return filteredCommits
}

func (g *GitRepo) checkInRevertCommit(c string, revert_commits []string) bool {
	for _, commit := range revert_commits {
		if strings.Contains(commit, c) {
			return true
		}
	}
	return false
}

// CreateMergeRequest 创建Mr
func (g *GitRepo) CreateMergeRequest(title, src, target string) (mrInfo *model.MrInfo, err error) {
	var (
		gitClient *gitlab.Client
		mr        *gitlab.MergeRequest
		resSet    []*gitlab.MergeRequest
	)
	defer func() {
		if err != nil {
			logrus.Debugf("CreateMergeRequest err:%s", err)
		}
	}()

	if gitClient, err = gitlab.NewClient(g.gitlabConfig.Token, gitlab.WithBaseURL(g.gitlabConfig.BaseUrl)); err != nil {
		return
	}

	//查询
	if resSet, _, err = gitClient.MergeRequests.ListProjectMergeRequests(g.getPid(), &gitlab.ListProjectMergeRequestsOptions{
		State:        stringPtr("opened"),
		SourceBranch: stringPtr(src),
		TargetBranch: stringPtr(target),
	}); err != nil {
		return
	}

	if len(resSet) > 0 {
		mrInfo = &model.MrInfo{
			Title:  resSet[0].Title,
			MrId:   resSet[0].IID,
			WebUrl: resSet[0].WebURL,
		}
		return
	}

	if mr, _, err = gitClient.MergeRequests.CreateMergeRequest(g.getPid(), &gitlab.CreateMergeRequestOptions{
		Title:              stringPtr(title),
		Description:        stringPtr(title),
		SourceBranch:       stringPtr(src),
		TargetBranch:       stringPtr(target),
		RemoveSourceBranch: boolPtr(true),
		Squash:             boolPtr(true),
	}); err != nil {
		return
	}

	mrInfo = &model.MrInfo{
		Title:  title,
		MrId:   mr.IID,
		WebUrl: mr.WebURL,
	}
	return
}

func (g *GitRepo) AcceptMergeRequest(mrId int) (res string, err error) {
	var (
		gitClient *gitlab.Client
	)
	defer func() {
		if err != nil {
			logrus.Debugf("CreateMergeRequest err:%s", err)
			err = nil
			res = MergeResFail
		}
	}()

	if gitClient, err = gitlab.NewClient(g.gitlabConfig.Token, gitlab.WithBaseURL(g.gitlabConfig.BaseUrl)); err != nil {
		return
	}

	res = MergeResOk
	if _, _, err = gitClient.MergeRequests.AcceptMergeRequest(g.getPid(), mrId, nil); err != nil {
		logrus.Debugf("直接合并失败，mrId:%d,错误原因:%v", mrId, err)

		mergeSuccess := false
		for i := 0; i < 3; i++ {
			time.Sleep(5 * time.Second)
			logrus.Debugf("正在第%d/3次尝试重新合并%d", i+1, mrId)
			if _, _, err = gitClient.MergeRequests.AcceptMergeRequest(g.getPid(), mrId, nil); err == nil {
				mergeSuccess = true
				break
			} else {
				logrus.Debugf("正在第%d/3次尝试重新合并%d，错误：%v", i+1, mrId, err)
			}

		}

		if !mergeSuccess {
			//无法立即合并，改由ci完成后合并
			logrus.Debugf("直接合并失败，正在尝试等待pipeline完成后合并%d", mrId)
			for i := 0; i < 3; i++ {
				time.Sleep(5 * time.Second)
				logrus.Debugf("正在第%d/3次尝试重新合并%d", i+1, mrId)
				if _, _, err = gitClient.MergeRequests.AcceptMergeRequest(g.getPid(), mrId, &gitlab.AcceptMergeRequestOptions{MergeWhenPipelineSucceeds: boolPtr(true)}); err == nil {
					res = MergeResWaitPipeline
					mergeSuccess = true
					break
				} else {
					logrus.Debugf("正在第%d/3次尝试重新合并%d，错误：%v", i+1, mrId, err)
				}
			}
		}

		if !mergeSuccess {
			res = MergeResFail
		}

	}
	return
}

func (g *GitRepo) GetMergeRequest(mrId int) (mr *gitlab.MergeRequest, err error) {
	var (
		gitClient *gitlab.Client
	)
	if gitClient, err = gitlab.NewClient(g.gitlabConfig.Token, gitlab.WithBaseURL(g.gitlabConfig.BaseUrl)); err != nil {
		return
	}

	mr, _, err = gitClient.MergeRequests.GetMergeRequest(g.getPid(), mrId, nil)
	return
}

// AutoJiraID 获取目录下的第一个git log 解析出JIRA-ID
func AutoJiraID(dir string, jiraProjects []string, commitID string) (JiraID string, commitType, commitMsg string) {
	var (
		err error
	)

	args := []string{"log", "--pretty=format:%s", "-n 1"}
	if commitID != "" {
		args = append(args, commitID)
	}

	cmdRet, err := ExecCmd(dir, "git", args...)
	if err != nil {
		logrus.Debugf("get jira commits faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return "", "", ""
	}
	lines := cmdRet.Out
	lines = strings.Trim(lines, " ")
	if len(lines) == 0 {
		return "", "", ""
	}

	//配置了jira项目前缀
	for _, p := range jiraProjects {
		re := regexp.MustCompile(fmt.Sprintf(`%s-\w+`, p))
		// 查找匹配项
		if match := re.FindString(lines); match != "" {
			return match, model.CommitTypeJira, ""
		}
	}

	//尝试用正则来找
	re := regexp.MustCompile(`\b[A-Z]+-\d+\b`)
	// 查找匹配项
	if match := re.FindString(lines); match != "" {
		return match, model.CommitTypeJira, ""
	}

	//无法匹配则以整个message作为jira
	hasher := md5.New()
	hasher.Write([]byte(lines))
	hash := hasher.Sum(nil)
	hashString := hex.EncodeToString(hash)

	return hashString, model.CommitTypeMsg, lines
}

// AutoBranch 自动根据当前的目录
func AutoBranch(dir string) string {
	cmdRet, err := ExecCmd(dir, "git", "branch", "--show-current")
	if err != nil {
		logrus.Debugf("get jira commits faild: out: %s, err: %s \n", cmdRet.Out, cmdRet.ErrStr)
		return ""
	}
	lines := strings.Split(cmdRet.Out, "\n")
	for _, l := range lines {
		s := strings.Trim(l, " ")
		if s != "" {
			return s
		}
	}
	return ""
}

func (g *GitRepo) getPid() string {
	projectPath := strings.Replace(g.Url, g.gitlabConfig.BaseUrl, "", 1)
	projectPath = strings.Trim(projectPath, "/")

	return projectPath
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
