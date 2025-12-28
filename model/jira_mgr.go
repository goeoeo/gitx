package model

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/goeoeo/gitx/util"

	_ "github.com/mattn/go-sqlite3"
)

type JiraMgr struct {
	saveDir  string
	dbPath   string
	db       *sql.DB
	JiraList []*Jira
}

func NewJiraMgr() (jm *JiraMgr, err error) {
	d, _ := os.UserHomeDir()
	jm = &JiraMgr{
		saveDir: filepath.Join(d, ".patch"),
		dbPath:  filepath.Join(d, ".patch", "gitx.db"),
	}

	// 初始化数据库
	if err = jm.initDB(); err != nil {
		return
	}

	// 数据迁移：从旧的JSON文件迁移到数据库
	if err = jm.migrateFromJSON(); err != nil {
		return
	}

	if err = jm.load(); err != nil {
		return
	}

	for _, v := range jm.JiraList {
		v.Init()
	}
	return
}

func (jm *JiraMgr) initDB() (err error) {
	// 打开数据库连接
	jm.db, err = sql.Open("sqlite3", jm.dbPath)
	if err != nil {
		return fmt.Errorf("open db error: %v", err)
	}

	// 创建表结构
	queries := []string{
		// jira表
		`CREATE TABLE IF NOT EXISTS jira (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project TEXT NOT NULL,
			jira_id TEXT NOT NULL,
			commit_type TEXT NOT NULL,
			commit_message TEXT,
			merged INTEGER NOT NULL DEFAULT 0,
			create_time DATETIME NOT NULL,
			update_time DATETIME NOT NULL,
			UNIQUE(project, jira_id)
		)`,
		// jira_branch表
		`CREATE TABLE IF NOT EXISTS jira_branch (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			jira_id INTEGER NOT NULL,
			branch_name TEXT,
			dev_branch TEXT,
			target_branch TEXT NOT NULL,
			merged INTEGER NOT NULL DEFAULT 0,
			create_time DATETIME NOT NULL,
			update_time DATETIME NOT NULL,
			FOREIGN KEY(jira_id) REFERENCES jira(id) ON DELETE CASCADE
		)`,
		// commit_info表
		`CREATE TABLE IF NOT EXISTS commit_info (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			jira_branch_id INTEGER NOT NULL,
			commit_id TEXT NOT NULL,
			desc TEXT,
			target_exists INTEGER NOT NULL DEFAULT 0,
			create_time DATETIME NOT NULL,
			FOREIGN KEY(jira_branch_id) REFERENCES jira_branch(id) ON DELETE CASCADE
		)`,
		// mr_info表
		`CREATE TABLE IF NOT EXISTS mr_info (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			jira_branch_id INTEGER NOT NULL,
			title TEXT,
			mr_id INTEGER,
			web_url TEXT,
			FOREIGN KEY(jira_branch_id) REFERENCES jira_branch(id) ON DELETE CASCADE
		)`,
		// link_info表
		`CREATE TABLE IF NOT EXISTS link_info (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			jira_branch_id INTEGER NOT NULL,
			link_type TEXT,
			issue_id TEXT,
			summary TEXT,
			status TEXT,
			UNIQUE(jira_branch_id),
			FOREIGN KEY(jira_branch_id) REFERENCES jira_branch(id) ON DELETE CASCADE
		)`,
		// jira_target_branch表（用于存储多对多关系）
		`CREATE TABLE IF NOT EXISTS jira_target_branch (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			jira_id INTEGER NOT NULL,
			target_branch TEXT NOT NULL,
			FOREIGN KEY(jira_id) REFERENCES jira(id) ON DELETE CASCADE,
			UNIQUE(jira_id, target_branch)
		)`,
	}

	for _, query := range queries {
		_, err = jm.db.Exec(query)
		if err != nil {
			return fmt.Errorf("create table error: %v", err)
		}
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

// migrateFromJSON 从旧的JSON文件迁移数据到数据库
func (jm *JiraMgr) migrateFromJSON() (err error) {
	// 检查是否已经迁移过（通过检查是否有数据）
	var count int
	row := jm.db.QueryRow("SELECT COUNT(*) FROM jira")
	if err = row.Scan(&count); err != nil {
		return fmt.Errorf("check migration status error: %v", err)
	}

	// 已有数据，跳过迁移
	if count > 0 {
		return
	}

	// 读取旧的JSON文件
	oldPath := filepath.Join(jm.saveDir, "jira.json")
	if _, err = os.Stat(oldPath); err != nil {
		// 文件不存在，不需要迁移
		return nil
	}

	b, err := ioutil.ReadFile(oldPath)
	if err != nil {
		return fmt.Errorf("read old jira.json error: %v", err)
	}

	if len(b) == 0 {
		return
	}

	// 解析旧数据
	var oldJiraList []*Jira
	if err = json.Unmarshal(b, &oldJiraList); err != nil {
		return fmt.Errorf("unmarshal old jira.json error: %v", err)
	}

	// 开始事务
	tx, err := jm.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction error: %v", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	// 插入数据
	for _, jira := range oldJiraList {
		// 插入jira
		result, err := tx.Exec(
			"INSERT INTO jira (project, jira_id, commit_type, commit_message, merged, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?, ?)",
			jira.Project, jira.JiraID, jira.CommitType, jira.CommitMessage, jira.Merged, jira.CreateTime, jira.UpdateTime,
		)
		if err != nil {
			return fmt.Errorf("insert jira error: %v", err)
		}

		jiraID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("get jira id error: %v", err)
		}

		// 插入target branch
		for _, branch := range jira.TargetBranch {
			_, err = tx.Exec(
				"INSERT INTO jira_target_branch (jira_id, target_branch) VALUES (?, ?)",
				jiraID, branch,
			)
			if err != nil {
				return fmt.Errorf("insert jira_target_branch error: %v", err)
			}
		}

		// 插入jira branch
		for _, jb := range jira.BranchList {
			result, err := tx.Exec(
				"INSERT INTO jira_branch (jira_id, branch_name, dev_branch, target_branch, merged, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?, ?)",
				jiraID, jb.BranchName, jb.DevBranch, jb.TargetBranch, jb.Merged, jb.CreateTime, jb.UpdateTime,
			)
			if err != nil {
				return fmt.Errorf("insert jira_branch error: %v", err)
			}

			jbID, err := result.LastInsertId()
			if err != nil {
				return fmt.Errorf("get jira_branch id error: %v", err)
			}

			// 插入commit info
			for _, ci := range jb.Commits {
				_, err = tx.Exec(
					"INSERT INTO commit_info (jira_branch_id, commit_id, desc, target_exists, create_time) VALUES (?, ?, ?, ?, ?)",
					jbID, ci.CommitId, ci.Desc, ci.TargetExists, ci.CreateTime,
				)
				if err != nil {
					return fmt.Errorf("insert commit_info error: %v", err)
				}
			}

			// 插入mr info
			for _, mr := range jb.MergeRequests {
				_, err = tx.Exec(
					"INSERT INTO mr_info (jira_branch_id, title, mr_id, web_url) VALUES (?, ?, ?, ?)",
					jbID, mr.Title, mr.MrId, mr.WebUrl,
				)
				if err != nil {
					return fmt.Errorf("insert mr_info error: %v", err)
				}
			}

			// 插入link info
			if jb.LinkInfo != nil {
				_, err = tx.Exec(
					"INSERT INTO link_info (jira_branch_id, link_type, issue_id, summary, status) VALUES (?, ?, ?, ?, ?)",
					jbID, jb.LinkInfo.LinkType, jb.LinkInfo.IssueId, jb.LinkInfo.Summary, jb.LinkInfo.Status,
				)
				if err != nil {
					return fmt.Errorf("insert link_info error: %v", err)
				}
			}
		}
	}

	return
}

func (jm *JiraMgr) load() (err error) {
	// 清空当前列表
	jm.JiraList = []*Jira{}

	// 查询所有jira
	rows, err := jm.db.Query("SELECT id, project, jira_id, commit_type, commit_message, merged, create_time, update_time FROM jira")
	if err != nil {
		return fmt.Errorf("query jira error: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var j Jira
		var id int64
		if err = rows.Scan(&id, &j.Project, &j.JiraID, &j.CommitType, &j.CommitMessage, &j.Merged, &j.CreateTime, &j.UpdateTime); err != nil {
			return fmt.Errorf("scan jira error: %v", err)
		}

		// 查询target branch
		branchRows, err := jm.db.Query("SELECT target_branch FROM jira_target_branch WHERE jira_id = ?", id)
		if err != nil {
			return fmt.Errorf("query target branch error: %v", err)
		}
		for branchRows.Next() {
			var branch string
			if err = branchRows.Scan(&branch); err != nil {
				branchRows.Close()
				return fmt.Errorf("scan target branch error: %v", err)
			}
			j.TargetBranch = append(j.TargetBranch, branch)
		}
		branchRows.Close()

		// 查询jira branch
		jbRows, err := jm.db.Query("SELECT id, branch_name, dev_branch, target_branch, merged, create_time, update_time FROM jira_branch WHERE jira_id = ?", id)
		if err != nil {
			return fmt.Errorf("query jira branch error: %v", err)
		}

		for jbRows.Next() {
			var jb JiraBranch
			var jbID int64
			if err = jbRows.Scan(&jbID, &jb.BranchName, &jb.DevBranch, &jb.TargetBranch, &jb.Merged, &jb.CreateTime, &jb.UpdateTime); err != nil {
				jbRows.Close()
				return fmt.Errorf("scan jira branch error: %v", err)
			}

			// 查询commit info
			ciRows, err := jm.db.Query("SELECT commit_id, desc, target_exists, create_time FROM commit_info WHERE jira_branch_id = ?", jbID)
			if err != nil {
				jbRows.Close()
				return fmt.Errorf("query commit info error: %v", err)
			}

			for ciRows.Next() {
				var ci CommitInfo
				if err = ciRows.Scan(&ci.CommitId, &ci.Desc, &ci.TargetExists, &ci.CreateTime); err != nil {
					ciRows.Close()
					jbRows.Close()
					return fmt.Errorf("scan commit info error: %v", err)
				}
				jb.Commits = append(jb.Commits, &ci)
			}
			ciRows.Close()

			// 查询mr info
			mrRows, err := jm.db.Query("SELECT title, mr_id, web_url FROM mr_info WHERE jira_branch_id = ?", jbID)
			if err != nil {
				jbRows.Close()
				return fmt.Errorf("query mr info error: %v", err)
			}

			for mrRows.Next() {
				var mr MrInfo
				if err = mrRows.Scan(&mr.Title, &mr.MrId, &mr.WebUrl); err != nil {
					mrRows.Close()
					jbRows.Close()
					return fmt.Errorf("scan mr info error: %v", err)
				}
				jb.MergeRequests = append(jb.MergeRequests, &mr)
			}
			mrRows.Close()

			// 查询link info
			linkRows, err := jm.db.Query("SELECT link_type, issue_id, summary, status FROM link_info WHERE jira_branch_id = ?", jbID)
			if err != nil {
				jbRows.Close()
				return fmt.Errorf("query link info error: %v", err)
			}

			if linkRows.Next() {
				var li LinkInfoItem
				if err = linkRows.Scan(&li.LinkType, &li.IssueId, &li.Summary, &li.Status); err != nil {
					linkRows.Close()
					jbRows.Close()
					return fmt.Errorf("scan link info error: %v", err)
				}
				jb.LinkInfo = &li
			}
			linkRows.Close()

			j.BranchList = append(j.BranchList, &jb)
		}
		jbRows.Close()

		jm.JiraList = append(jm.JiraList, &j)
	}

	return
}

func (jm *JiraMgr) Save() (err error) {
	// 开始事务
	tx, err := jm.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction error: %v", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	// 清空所有数据
	if _, err = tx.Exec("DELETE FROM link_info"); err != nil {
		return fmt.Errorf("delete link_info error: %v", err)
	}
	if _, err = tx.Exec("DELETE FROM mr_info"); err != nil {
		return fmt.Errorf("delete mr_info error: %v", err)
	}
	if _, err = tx.Exec("DELETE FROM commit_info"); err != nil {
		return fmt.Errorf("delete commit_info error: %v", err)
	}
	if _, err = tx.Exec("DELETE FROM jira_branch"); err != nil {
		return fmt.Errorf("delete jira_branch error: %v", err)
	}
	if _, err = tx.Exec("DELETE FROM jira_target_branch"); err != nil {
		return fmt.Errorf("delete jira_target_branch error: %v", err)
	}
	if _, err = tx.Exec("DELETE FROM jira"); err != nil {
		return fmt.Errorf("delete jira error: %v", err)
	}

	// 重新插入数据
	for _, jira := range jm.JiraList {
		// 插入jira
		result, err := tx.Exec(
			"INSERT INTO jira (project, jira_id, commit_type, commit_message, merged, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?, ?)",
			jira.Project, jira.JiraID, jira.CommitType, jira.CommitMessage, jira.Merged, jira.CreateTime, jira.UpdateTime,
		)
		if err != nil {
			return fmt.Errorf("insert jira error: %v", err)
		}

		jiraID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("get jira id error: %v", err)
		}

		// 插入target branch
		for _, branch := range jira.TargetBranch {
			_, err = tx.Exec(
				"INSERT INTO jira_target_branch (jira_id, target_branch) VALUES (?, ?)",
				jiraID, branch,
			)
			if err != nil {
				return fmt.Errorf("insert jira_target_branch error: %v", err)
			}
		}

		// 插入jira branch
		for _, jb := range jira.BranchList {
			result, err := tx.Exec(
				"INSERT INTO jira_branch (jira_id, branch_name, dev_branch, target_branch, merged, create_time, update_time) VALUES (?, ?, ?, ?, ?, ?, ?)",
				jiraID, jb.BranchName, jb.DevBranch, jb.TargetBranch, jb.Merged, jb.CreateTime, jb.UpdateTime,
			)
			if err != nil {
				return fmt.Errorf("insert jira_branch error: %v", err)
			}

			jbID, err := result.LastInsertId()
			if err != nil {
				return fmt.Errorf("get jira_branch id error: %v", err)
			}

			// 插入commit info
			for _, ci := range jb.Commits {
				_, err = tx.Exec(
					"INSERT INTO commit_info (jira_branch_id, commit_id, desc, target_exists, create_time) VALUES (?, ?, ?, ?, ?)",
					jbID, ci.CommitId, ci.Desc, ci.TargetExists, ci.CreateTime,
				)
				if err != nil {
					return fmt.Errorf("insert commit_info error: %v", err)
				}
			}

			// 插入mr info
			for _, mr := range jb.MergeRequests {
				_, err = tx.Exec(
					"INSERT INTO mr_info (jira_branch_id, title, mr_id, web_url) VALUES (?, ?, ?, ?)",
					jbID, mr.Title, mr.MrId, mr.WebUrl,
				)
				if err != nil {
					return fmt.Errorf("insert mr_info error: %v", err)
				}
			}

			// 插入link info
			if jb.LinkInfo != nil {
				_, err = tx.Exec(
					"INSERT INTO link_info (jira_branch_id, link_type, issue_id, summary, status) VALUES (?, ?, ?, ?, ?)",
					jbID, jb.LinkInfo.LinkType, jb.LinkInfo.IssueId, jb.LinkInfo.Summary, jb.LinkInfo.Status,
				)
				if err != nil {
					return fmt.Errorf("insert link_info error: %v", err)
				}
			}
		}
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
