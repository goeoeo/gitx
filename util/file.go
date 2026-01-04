package util

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FindOriginURL 从配置文件中提取 origin URL
func FindOriginURL(pwd string) (string, error) {
	// 获取当前目录的 .git/config 路径
	gitConfigPath := filepath.Join(pwd, ".git", "config")

	// 打开配置文件
	file, err := os.Open(gitConfigPath)
	if err != nil {
		return "", fmt.Errorf("无法打开文件: %v\n", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inOriginSection := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 检测 [remote "origin"] 部分
		if strings.HasPrefix(line, "[remote \"origin\"]") {
			inOriginSection = true
			continue
		}

		// 离开当前 section
		if strings.HasPrefix(line, "[") && inOriginSection {
			break
		}

		// 在 origin section 中查找 URL
		if inOriginSection && strings.HasPrefix(line, "url") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				url := convertGitToHTTP(strings.TrimSpace(parts[1]))
				url = strings.TrimSuffix(url, ".git")
				return url, nil
			}
		}
	}

	return "", fmt.Errorf("查找远程URL失败")
}

// 将 Git 协议 URL 转换为 HTTP(S)
func convertGitToHTTP(url string) string {
	// 匹配 SSH 格式 (git@host:path)
	sshPattern := regexp.MustCompile(`^git@([\w.-]+):(.+\.git)$`)
	if matches := sshPattern.FindStringSubmatch(url); matches != nil {
		return fmt.Sprintf("https://%s/%s", matches[1], matches[2])
	}

	// 匹配 git:// 协议
	gitProtocolPattern := regexp.MustCompile(`^git://([\w.-]+)/(.+\.git)$`)
	if matches := gitProtocolPattern.FindStringSubmatch(url); matches != nil {
		return fmt.Sprintf("https://%s/%s", matches[1], matches[2])
	}

	// 已经是 HTTP 或无法转换
	return url
}

func ReadJsonFile(f string, v any) (err error) {
	var (
		c []byte
	)

	if !FileExists(f) {
		return
	}

	if c, err = os.ReadFile(f); err != nil {
		return
	}

	if err = json.Unmarshal(c, v); err != nil {
		return
	}
	return
}

func WriteJsonFile(f string, v any) (err error) {
	var (
		c []byte
	)

	if c, err = json.MarshalIndent(v, "", "  "); err != nil {
		return
	}
	if err = os.WriteFile(f, c, os.ModePerm); err != nil {
		return
	}

	return
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
