package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/goeoeo/gitx/repo"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var BranchDelCmd = &cobra.Command{
	Use:   "branchDel",
	Short: "删除本地和远程分支",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			logrus.Errorf("请提供分支关键词")
			return
		}
		keyword := args[0]

		// 获取当前目录
		currentDir, err := os.Getwd()
		if err != nil {
			logrus.Errorf("获取当前目录失败: %v", err)
			return
		}

		// 创建 GitRepo 对象
		gitRepo := repo.NewGitRepo(currentDir, "")

		// 获取所有本地分支
		localBranches, err := gitRepo.GetBranchs()
		if err != nil {
			logrus.Errorf("获取本地分支失败: %v", err)
			return
		}

		// 模糊匹配本地分支
		var matchedBranches []string
		for _, branch := range localBranches {
			if strings.Contains(branch, keyword) {
				matchedBranches = append(matchedBranches, branch)
			}
		}

		if len(matchedBranches) == 0 {
			logrus.Infof("没有匹配到分支")
			return
		}

		// 列出匹配的分支
		fmt.Println("匹配到的本地分支:")
		for _, branch := range matchedBranches {
			fmt.Printf("- %s\n", branch)
		}

		// 交互式确认
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("确认删除以上所有分支吗？(y/n): ")
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(confirm)

		if strings.ToLower(confirm) != "y" {
			logrus.Infof("取消删除操作")
			return
		}

		// 删除分支
		for _, branch := range matchedBranches {
			// 删除本地分支
			fmt.Printf("正在删除本地分支: %s\n", branch)
			err := gitRepo.DelLocalBranch(branch)
			if err != nil {
				logrus.Warnf("删除本地分支 %s 失败: %v", branch, err)
			} else {
				logrus.Infof("删除本地分支 %s 成功", branch)
			}

			// 删除远程分支
			fmt.Printf("正在删除远程分支: %s\n", branch)
			err = gitRepo.DelRemoteBranch(branch)
			if err != nil {
				logrus.Warnf("删除远程分支 %s 失败: %v", branch, err)
			} else {
				logrus.Infof("删除远程分支 %s 成功", branch)
			}
		}
	},
}

func init() {
	// 可以添加命令行参数
}