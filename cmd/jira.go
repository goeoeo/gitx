package cmd

import (
	"fmt"
	"github.com/goeoeo/gitx/controller"
	"github.com/goeoeo/gitx/repo"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
)

var JiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "jira",
	Run: func(cmd *cobra.Command, args []string) {
		config := repo.GetConfig(configPath).Init()
		jc, err := controller.NewJiraController(config)
		config.CheckErr(err)
		if project == "" {
			project = config.Patch.CurrentProject
		}

		fmt.Println("current project:", project)
		switch action {
		case "add":
			err = jc.Add(project, jiraID, strings.Split(branchList, ","))
		case "del":
			err = jc.Del(project, jiraID)
		case "clear":
			err = jc.Clear(project, disableCheckMerged)
		case "print":
			err = jc.Print(project, jiraID)
		default:
			err = fmt.Errorf("action not suppert:%s", action)
		}

		config.CheckErr(err)

		fmt.Printf("%s success\n", action)
	},
}

func init() {
	JiraCmd.Flags().StringVarP(&configPath, "config", "c", defaultConfigPath(), "配置文件路径")
	JiraCmd.Flags().StringVarP(&action, "action", "a", "", "方法:add,del,clear")
	JiraCmd.Flags().StringVarP(&project, "project", "p", "", "项目")
	JiraCmd.Flags().StringVarP(&jiraID, "jiraId", "j", "", "jiraID")
	JiraCmd.Flags().StringVarP(&branchList, "branchList", "b", "", "目标分支，支持逗号分隔")
	JiraCmd.PersistentFlags().BoolVarP(&disableCheckMerged, "disableCheckMerged", "d", false, "删除临时分支前是否检查已经合并")

}

func defaultConfigPath() string {
	d, _ := os.UserHomeDir()
	return filepath.Join(d, ".patch", "config.yaml")
}
