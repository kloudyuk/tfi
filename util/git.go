package util

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/xanzy/go-gitlab"
)

var gitlabClient *gitlab.Client
var repo *git.Repository

const varPrefix = "TF_VAR_"

func Git() *git.Repository {
	if repo != nil {
		return repo
	}
	d, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	repo, err = git.PlainOpenWithOptions(d, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		panic(err)
	}
	return repo
}

func Gitlab() *gitlab.Client {
	if gitlabClient != nil {
		return gitlabClient
	}
	token, ok := os.LookupEnv("GITLAB_TOKEN")
	if !ok {
		token, ok = os.LookupEnv("CI_JOB_TOKEN")
		if !ok {
			panic("Gitlab token not found")
		}
	}
	var err error
	gitlabClient, err = gitlab.NewClient(token)
	if err != nil {
		panic(err)
	}
	return gitlabClient
}

func GitlabProject() *gitlab.Project {
	remote, err := Git().Remote("origin")
	if err != nil {
		panic(err)
	}
	url := remote.Config().URLs[0]
	path := strings.TrimPrefix(url, "git@gitlab.com:")
	path = strings.TrimSuffix(path, ".git")
	project, _, err := Gitlab().Projects.GetProject(path, &gitlab.GetProjectOptions{})
	if err != nil {
		panic(err)
	}
	return project
}

func GitlabVars() (map[string]string, error) {
	git := Gitlab()
	project := GitlabProject()
	groups := strings.Split(project.PathWithNamespace, "/")
	groups = groups[0 : len(groups)-1]
	groupPath := ""
	varMap := map[string]string{}
	for _, v := range groups {
		groupPath = filepath.Join(groupPath, v)
		opt := &gitlab.ListGroupVariablesOptions{
			Page:    1,
			PerPage: 100,
		}
		for {
			vars, resp, err := git.GroupVariables.ListVariables(groupPath, opt)
			if err != nil {
				return nil, err
			}
			if len(vars) > 0 {
				for _, v := range vars {
					if strings.HasPrefix(v.Key, varPrefix) {
						varMap[strings.TrimPrefix(v.Key, varPrefix)] = v.Value
					}
				}
			}
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
	}
	opt := &gitlab.ListProjectVariablesOptions{
		Page:    1,
		PerPage: 100,
	}
	for {
		vars, resp, err := git.ProjectVariables.ListVariables(project.ID, opt)
		if err != nil {
			return nil, err
		}
		if len(vars) > 0 {
			for _, v := range vars {
				varMap[v.Key] = v.Value
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return varMap, nil
}
