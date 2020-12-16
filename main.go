package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/bitrise-io/go-steputils/stepconf"
	"github.com/bitrise-io/go-utils/log"
	"github.com/bitrise-steplib/steps-git-clone/gitclone"
)

func failf(format string, v ...interface{}) {
	log.Errorf(format, v...)
	os.Exit(1)
}

type config struct {
	RepositoryURL string `env:"repository_url,required"`
	CloneIntoDir  string `env:"clone_into_dir,required"`
	Commit        string `env:"commit"`
	Tag           string `env:"tag"`
	Branch        string `env:"branch"`

	BranchDest      string `env:"branch_dest"`
	PRID            int    `env:"pull_request_id"`
	PRRepositoryURL string `env:"pull_request_repository_url"`
	PRMergeBranch   string `env:"pull_request_merge_branch"`
	ResetRepository bool   `env:"reset_repository,opt[Yes,No]"`
	CloneDepth      int    `env:"clone_depth"`

	BuildURL         string `env:"build_url"`
	BuildAPIToken    string `env:"build_api_token"`
	UpdateSubmodules bool   `env:"update_submodules,opt[yes,no]"`
	ManualMerge      bool   `env:"manual_merge,opt[yes,no]"`

	SSLVerify bool   `env:"ssl_verify,opt[yes,no]"`
	HTTPUser  string `env:"http_user"`
	HTTPToken string `env:"http_token"`
}

func printLogAndExportEnv(gitCmd git.Git, format, env string) error {
	l, err := output(gitCmd.Log(format))
	if err != nil {
		return err
	}

	log.Printf("=> %s\n   value: %s\n", env, l)
	if err := exportEnvironmentWithEnvman(env, l); err != nil {
		return fmt.Errorf("envman export, error: %v", err)
	}
	return nil
}

func exportEnvironmentWithEnvman(keyStr, valueStr string) error {
	cmd := command.New("envman", "add", "--key", keyStr)
	cmd.SetStdin(strings.NewReader(valueStr))
	return cmd.Run()
}

func main() {

	var cfg gitclone.Config

	if len(cfg.HTTPUser) > 0 && len(cfg.HTTPToken) > 0 && strings.HasPrefix(cfg.RepositoryURL, "http") {
		components := strings.SplitAfter(cfg.RepositoryURL, "://")
		cfg.RepositoryURL = components[0] + cfg.HTTPUser + ":" + cfg.HTTPToken + "@" + components[1]
		log.Infof("updated repo url: %s \n", cfg.RepositoryURL)
	}

	if !cfg.SSLVerify {
		if err := run(gitCmd.Config("http.sslVerify", "false")); err != nil {
			return fmt.Errorf("set config http.sslVerify false, error: %v", err)
		}
	}

	if err := stepconf.Parse(&cfg); err != nil {
		failf("Error: %s\n", err)
	}
	stepconf.Print(cfg)

	if err := gitclone.Execute(cfg); err != nil {
		failf("ERROR: %v", err)
	}
	log.Donef("\nSuccess")
}
