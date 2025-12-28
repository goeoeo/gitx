package main

import (
	"github.com/goeoeo/gitx/cmd"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{}

func main() {
	rootCmd.AddCommand(cmd.PushCmd, cmd.PullCmd, cmd.JiraCmd, cmd.InitCmd, cmd.InfoCmd, cmd.HookCmd, cmd.BranchDelCmd)
	if err := rootCmd.Execute(); err != nil {
		logrus.Debugf("run cmd err:%s", err)
	}
}
