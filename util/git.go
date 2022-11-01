package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/xanzy/go-gitlab"
)

var gitlabClient *gitlab.Client
var gitlabProject *gitlab.Project
var gitlabVars map[string]string
var repo *git.Repository

const varPrefix = "TF_VAR_"

func Git() (*git.Repository, error) {
	if repo != nil {
		return repo, nil
	}
	d, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	repo, err = git.PlainOpenWithOptions(d, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func Gitlab() (*gitlab.Client, error) {
	if gitlabClient != nil {
		return gitlabClient, nil
	}
	token := os.Getenv("GITLAB_TOKEN")
	var err error
	gitlabClient, err = gitlab.NewClient(token)
	if err != nil {
		return nil, err
	}
	return gitlabClient, nil
}

func GitlabProject() (*gitlab.Project, error) {
	if gitlabProject != nil {
		return gitlabProject, nil
	}
	fmt.Fprintln(os.Stderr, "Fetching Gitlab project info")
	git, err := Git()
	if err != nil {
		return nil, err
	}
	remote, err := git.Remote("origin")
	if err != nil {
		return nil, err
	}
	url := remote.Config().URLs[0]
	path := strings.TrimPrefix(url, "git@gitlab.com:")
	path = strings.TrimSuffix(path, ".git")
	gclient, err := Gitlab()
	if err != nil {
		return nil, err
	}
	gitlabProject, _, err = gclient.Projects.GetProject(path, &gitlab.GetProjectOptions{})
	if err != nil {
		return nil, err
	}
	return gitlabProject, nil
}

func GitlabVars() (map[string]string, error) {
	fmt.Fprintln(os.Stderr, "Getting Gitlab variables")
	if gitlabVars != nil {
		return gitlabVars, nil
	}
	gitlabVars = map[string]string{}
	gclient, err := Gitlab()
	if err != nil {
		return nil, err
	}
	project, err := GitlabProject()
	if err != nil {
		return nil, err
	}
	groups := strings.Split(project.PathWithNamespace, "/")
	groups = groups[0 : len(groups)-1]
	groupPath := ""
	for _, v := range groups {
		groupPath = filepath.Join(groupPath, v)
		opt := &gitlab.ListGroupVariablesOptions{
			Page:    1,
			PerPage: 100,
		}
		for {
			vars, resp, err := gclient.GroupVariables.ListVariables(groupPath, opt)
			if err != nil {
				return nil, err
			}
			if len(vars) > 0 {
				for _, v := range vars {
					if strings.HasPrefix(v.Key, varPrefix) {
						gitlabVars[strings.TrimPrefix(v.Key, varPrefix)] = v.Value
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
		vars, resp, err := gclient.ProjectVariables.ListVariables(project.ID, opt)
		if err != nil {
			return nil, err
		}
		if len(vars) > 0 {
			for _, v := range vars {
				gitlabVars[v.Key] = v.Value
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return gitlabVars, nil
}
