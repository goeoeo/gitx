package controller

import (
	"github.com/goeoeo/gitx/repo"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestJiraController_AddJira(t *testing.T) {
	cfg := repo.GetConfig("../config.yaml")
	jira, err := NewJiraController(cfg)
	assert.Nil(t, err)

	err = jira.Add("dev-tool", "VM-2074", []string{"dev", "qa", "staging"})
	assert.Nil(t, err)
}

func TestJiraController_DelJira(t *testing.T) {
	cfg := repo.GetConfig("../config.yaml")
	jira, err := NewJiraController(cfg)
	assert.Nil(t, err)

	err = jira.Del("dev-tool", "VM-2074")
	assert.Nil(t, err)
}

func TestJiraController_Clear(t *testing.T) {
	jira := getJiraController(t)
	err := jira.Clear()
	assert.Nil(t, err)
}

func TestJiraController_PrintJira(t *testing.T) {
	jira := getJiraController(t)
	err := jira.Print("production", "BILLING-3037")
	assert.Nil(t, err)
}

func TestJiraController_Detach(t *testing.T) {
	jira := getJiraController(t)

	err := jira.Detach("production", "BILLING-3383", "v6.2")
	assert.Nil(t, err)
}

func TestJiraController_CheckBranchMerged(t *testing.T) {
	jira := getJiraController(t)
	err := jira.CheckBranchMerged("production", "BILLING-3037")
	assert.Nil(t, err)
}

func getJiraController(t *testing.T) *JiraController {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{})
	cfg := repo.GetConfig("/Users/yu/.patch/config.yaml")
	cfg.DisableInitLog = true
	cfg.Init()
	jira, err := NewJiraController(cfg)
	assert.Nil(t, err)
	return jira
}
