package main

/**
this program creates vault tokens and store them periodically in the build variables
of your gitlab repos

you need to use a vault client to be able to read secrets on your pipelines
*/

import (
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	vault "github.com/hashicorp/vault/api"
	"github.com/robfig/cron"
	"github.com/sirupsen/logrus"
	gitlab "github.com/xanzy/go-gitlab"
	yaml "gopkg.in/yaml.v2"
)

func CheckPanic(err error) {
	if err != nil {
		panic(err)
	}
}

func (gs *GlobalSpec) Load(file string) {
	data, err := ioutil.ReadFile(file)
	CheckPanic(err)
	err = yaml.Unmarshal(data, gs)
	CheckPanic(err)
	if gs.Default.RepositorySpec == nil {
		panic("RepositorySpec should be present in the default spec")
	}
	if gs.Default.RepositorySpec.BuildKey == "" {
		panic("BuildKey should be present in the default spec")
	}

	if gs.Default.RepositorySpec.Enabled == nil {
		panic("Enabled should be present in the default spec")
	}

	if gs.Default.RepositorySpec.RenewPeriod == "" {
		panic("RenewPeriod should be present in the default spec")
	}
	_, err = time.ParseDuration(gs.Default.RepositorySpec.RenewPeriod)
	CheckError(err)
	if gs.Default.TokenSpec == nil {
		panic("TokenSpec should be present in the default spec")
	}
	if gs.Groups == nil {
		return
	}

	for _, group := range gs.Groups {
		if group.Default.RepositorySpec == nil {
			group.Default.RepositorySpec = gs.Default.RepositorySpec
		}
		if group.Default.TokenSpec == nil {
			group.Default.TokenSpec = gs.Default.TokenSpec
		}
		if group.Default.RepositorySpec.BuildKey == "" {
			group.Default.RepositorySpec.BuildKey = gs.Default.RepositorySpec.BuildKey
		}
		if group.Default.RepositorySpec.Enabled == nil {
			group.Default.RepositorySpec.Enabled = gs.Default.RepositorySpec.Enabled
		}
		if group.Default.RepositorySpec.RenewPeriod == "" {
			group.Default.RepositorySpec.RenewPeriod = gs.Default.RepositorySpec.RenewPeriod
		}
		_, err = time.ParseDuration(group.Default.RepositorySpec.RenewPeriod)
		CheckError(err)
		for _, project := range group.Projects {
			if project.RepositorySpec == nil {
				project.RepositorySpec = group.Default.RepositorySpec
			}
			if project.TokenSpec == nil {
				project.TokenSpec = group.Default.TokenSpec
			}
			if project.RepositorySpec.BuildKey == "" {
				project.RepositorySpec.BuildKey = group.Default.RepositorySpec.BuildKey
			}
			if project.RepositorySpec.Enabled == nil {
				project.RepositorySpec.Enabled = group.Default.RepositorySpec.Enabled
			}
			if project.RepositorySpec.RenewPeriod == "" {
				project.RepositorySpec.RenewPeriod = group.Default.RepositorySpec.RenewPeriod
			}
			_, err = time.ParseDuration(project.RepositorySpec.RenewPeriod)
			CheckError(err)
		}
	}
}

func (gs *GlobalSpec) GetSpecForPath(path string) *LinkSpec {
	parts := strings.Split(path, "/")
	groupName, projectName := parts[0], parts[1]
	ls := gs.Default
	if group, ok := gs.Groups[groupName]; ok {
		ls = group.Default
		if project, ok := group.Projects[projectName]; ok {
			ls = project
		}
	}
	return ls
}

var Cron *cron.Cron

var CronMap map[int]bool
var DefaultGitlabConfig GitlabConfig

func GetDefaultGitlabConfig() GitlabConfig {
	baseUrl := "https://gitlab.com/api/v4"
	be := os.Getenv("GITLAB_BASE_URL")
	if be != "" {
		baseUrl = be
	}

	return GitlabConfig{BaseUrl: baseUrl, Token: os.Getenv("GITLAB_TOKEN")}
}
func init() {
	Cron = cron.New()
	CronMap = map[int]bool{}
	DefaultGitlabConfig = GetDefaultGitlabConfig()
}

func CheckError(err error) {
	if err != nil {
		logrus.Error(err)
	}
}

func (gs *GlobalSpec) UpdateCron(gc *gitlab.Client, vc *vault.Client) {
	//load all the repositories  across the gitlab instance
	groups, _, err := gc.Groups.ListGroups(nil)
	CheckError(err)
	for _, group := range groups {
		projects, _, err := gc.Groups.ListGroupProjects(group.ID, nil)
		CheckError(err)

		for _, project := range projects {
			path := project.PathWithNamespace
			link := gs.GetSpecForPath(path)
			//if the connection is not active continue
			if !*link.RepositorySpec.Enabled {
				continue
			}
			if _, ok := CronMap[project.ID]; ok {
				continue
			}
			CronMap[project.ID] = true
			secret, vaultErr := vc.Auth().Token().Create(link.TokenSpec)
			if vaultErr != nil {
				logrus.Error(vaultErr)
				return
			}
			UpdateToken(gc, project.ID, link.RepositorySpec.BuildKey, secret.Auth.ClientToken)

			Cron.AddFunc("@every "+link.RepositorySpec.RenewPeriod, func() {
				secret, vaultErr := vc.Auth().Token().Create(link.TokenSpec)
				if vaultErr != nil {
					logrus.Error(vaultErr)
					return
				}
				UpdateToken(gc, project.ID, link.RepositorySpec.BuildKey, secret.Auth.ClientToken)
			})
		}
	}
}

//UpdateToken update the build key value
func UpdateToken(gc *gitlab.Client, projectId int, buildKey string, token string) {
	_, _, err := gc.BuildVariables.UpdateBuildVariable(projectId, buildKey, token)
	CheckError(err)
	if err == nil {
		logrus.Info(buildKey + " Updated for project " + strconv.Itoa(projectId))
	}
	if err != nil {
		//try to create the variable
		_, _, err := gc.BuildVariables.CreateBuildVariable(projectId, buildKey, token)
		CheckError(err)
		if err == nil {
			logrus.Info(buildKey + " Created for project " + strconv.Itoa(projectId))
		}

	}
}

func main() {
	//Get vault client
	vc, err := vault.NewClient(vault.DefaultConfig())
	CheckPanic(err)

	//Get gitlab Client
	git := gitlab.NewClient(nil, DefaultGitlabConfig.Token)
	git.SetBaseURL(DefaultGitlabConfig.BaseUrl)

	//Start cron
	Cron.Start()
	gs := &GlobalSpec{}
	gs.Load(os.Getenv("VGL_CONFIG_PATH"))

	for {
		gs.UpdateCron(git, vc)
		//update the list of repo each 5 min
		time.Sleep(5 * time.Minute)
	}
}
