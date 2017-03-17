package main

import vault "github.com/hashicorp/vault/api"

//RepoSpec ..
type RepoSpec struct {
	//BuildKey key where the vault token will be stored
	BuildKey    string `yaml:"buildKey,omitempty"`
	//Enabled if is enable of not, by default is true
	Enabled     *bool `yaml:"enabled,omitempty"`
	//examples "1h", "30m", "30s"
	RenewPeriod string `yaml:"renewPeriod,omitempty"`
}

//LinkSpec defines a link between a vault token and a repository
type LinkSpec struct {
	RepositorySpec *RepoSpec                 `yaml:"repositorySpec,omitempty"`
	TokenSpec      *vault.TokenCreateRequest `yaml:"tokenSpec,omitempty"`
}

//GroupSpec define the default spec for a group
type GroupSpec struct {
	Default  *LinkSpec `yaml:"default,omitempty"`
	Projects map[string]*LinkSpec `yaml:"projects,omitempty"`
}
//GlobalSpec default policy
type GlobalSpec struct {
	Default *LinkSpec
	Groups  map[string]*GroupSpec
}

type GitlabConfig struct {
	Token   string
	BaseUrl string
}