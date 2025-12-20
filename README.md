# Gitx - 高效代码合并工具

一个基于 Go 语言开发的智能代码合并工具，通过自动化 cherry-pick 流程，将原本繁琐的多分支、多项目代码合并工作化繁为简，每周为开发者节约宝贵的时间。

## 📖 项目简介

Gitx 旨在解决日常开发中频繁出现的、涉及多个分支和项目的代码合并痛点。传统合并流程需要重复执行 7 个步骤（如检出分支、创建临时分支、cherry-pick 代码、推送、创建 MR 等），在多分支（如 dev、qa、staging、v6.0、v6.1、v6.2）和多项目（A、B、C）场景下，需执行 6×3=18 次重复操作，耗时 36+ 分钟。Gitx 通过一键命令，自动化完成所有这些步骤，将效率提升到极致。

## 🚀 核心功能

| 命令   | 作用                     |
|------|------------------------|
| push | cherry-pick 方式自动化推送合并代码 |
| hook | 通过 Hook 触发 Jenkins 等 CI/CD 工具 |
| info | 打印当前项目仓库信息             |
| jira | 管理和打印 Jira 相关提交信息       |
| init | 初始化项目配置文件             |
| pull | 拉取代码（补充完整命令说明）         |

## 💡 push 命令实现原理

核心思想：**以 Jira 任务为中心，批量自动化处理代码合并流程**

### 场景示例
- 开发分支：`VM-2003_Tpsc_Migrate_develop`
- 目标：合并到 `staging` 分支

### 执行流程
1. **拉取最新代码**：自动拉取目标分支（staging）的最新代码
2. **创建临时分支**：基于最新目标分支创建临时分支（`VM-2003_Tpsc_Migrate_staging`）
3. **智能 cherry-pick**：通过 Jira ID 匹配筛选相关 commit，自动合并到临时分支
4. **自动推送**：将临时分支推送到远程仓库
5. **自动创建 MR**：调用 GitLab API 自动创建 Merge Request
6. **自动合并 MR**：（可选）配置后可自动合并 MR

## 🛠️ 安装

### 方法 1：Go Install
```bash
go install github.com/goeoeo/gitx@latest
```

### 方法 2：直接下载二进制
```bash
sudo wget -O /usr/local/bin/gitx "https://github.com/goeoeo/gitx/releases/download/v0.0.1/gitx_$(uname -s)_$(uname -m)" && sudo chmod +x /usr/local/bin/gitx
```

### 注意事项
- **Mac 系统**：如果遇到无法打开的错误，需要在 `系统设置 -> 隐私与安全` 中授权

## 📝 使用指南

### 1. 初始化配置
```bash
gitx init
```

### 2. 编辑配置文件
```bash
vim ~/.patch/config.yaml
```

**Windows 系统**：
```bash
notepad %USERPROFILE%\.patch\config.yaml
```

### 3. 基础使用

#### 推送带 Jira 信息的 Commit
将当前分支中的 commit 推送到 `dev` 和 `qa` 分支：
```bash
gitx push -b dev,qa
```

指定 Jira ID 推送：
```bash
gitx push -b dev,qa -j VM-8888
```

#### 推送多个项目
```bash
gitx push -b dev,qa -p common,ws,fg
```

#### 推送不带 Jira 信息的 Commit
```bash
gitx push -b dev,qa
```

#### 清理临时分支
```bash
gitx jira -a clear
```

#### 查看帮助
```bash
gitx -h
```

### 4. 高级功能

#### 自动清理分支配置
可以将清理分支命令配置为定时任务：
```bash
crontab -e
```

添加以下内容，每天 17:05 自动清理：
```bash
5 17 * * * /usr/local/bin/gitx jira -a=clear
```

#### 处理冲突
1. 使用 IDE 解决冲突
2. 执行 `git cherry-pick --continue`
3. 输入 `y` 继续执行

## 🏗️ 项目架构

```
gitx/
├── main.go              # 程序入口
├── cmd/                 # 命令包
│   ├── push.go          # push 命令实现
│   ├── jira.go          # jira 命令实现
│   ├── init.go          # init 命令实现
│   ├── hook.go          # hook 命令实现
│   ├── info.go          # info 命令实现
│   └── config.yaml      # 配置模板
├── controller/          # 控制器层
│   └── jira.go
├── model/               # 数据模型
├── repo/                # Git 仓库操作
├── util/                # 工具函数
│   ├── git.go
│   └── util.go
├── LICENSE
├── Makefile
└── README.md
```

## 🛠️ 技术栈

- **语言**：Go
- **依赖**：
  - cobra：命令行框架
  - logrus：日志库
  - 原生 Git 命令集成
- **支持平台**：
  - 所有 Linux 发行版
  - macOS
  - Windows (待测试)

## 📊 效率对比

| 操作场景                | 手动操作耗时 | Gitx 操作耗时 | 效率提升 |
|---------------------|---------|------------|------|
| 单项目+单分支合并          | 2 分钟    | 10 秒       | 12x  |
| 多项目（A,B,C）+多分支（v6.0,v6.1,v6.2） | 36 分钟   | 2 分钟      | 18x  |

## 🤝 贡献指南

1. Fork 项目
2. 创建功能分支
3. 提交更改
4. 推送到远程分支
5. 创建 Pull Request

## 📝 许可证

MIT License

## 📞 支持

如果您有问题或建议，请在 [GitHub Issues](https://github.com/goeoeo/gitx/issues) 中提出。

---

**如果这个项目帮助到了你，请给一个 Star ⭐！**