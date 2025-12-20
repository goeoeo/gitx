package cmd

import (
	_ "embed"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/goeoeo/gitx/repo"
	"github.com/spf13/cobra"
)

// configContentTpl 配置文件模板
//
//go:embed config.yaml
var configContentTpl string
var try bool
var InitCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化项目配置文件",
	Run: func(cmd *cobra.Command, args []string) {
		if try {
			config := repo.GetConfig()
			config.Print()

			return
		}
		err := createPatchConfig()
		checkErr(err)

		return
	},
}

func init() {
	InitCmd.Flags().BoolVarP(&try, "try", "t", false, "打印配置文件")

}
func createPatchConfig() (err error) {
	var (
		homeDir string
	)

	if homeDir, err = os.UserHomeDir(); err != nil {
		return
	}

	dir := filepath.Join(homeDir, ".patch")
	if _, err = os.Stat(dir); err != nil {
		if err = os.MkdirAll(dir, os.ModePerm); err != nil {
			return
		}
	}

	configPath := filepath.Join(homeDir, ".patch", "config.yaml")

	// 文件不存在，创建
	if _, err = os.Stat(configPath); err == nil {
		fmt.Println("已存在配置：", configPath)
		return
	}

	fmt.Println("请配置你的信息，路径为：", configPath)

	err = ioutil.WriteFile(configPath, []byte(configContentTpl), os.ModePerm)
	return
}
