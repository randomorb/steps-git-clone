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
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Error: %s\n", err)
	}
	stepconf.Print(cfg)

	if err := gitclone.Execute(cfg); err != nil {
		failf("ERROR: %v", err)
	}

	if len(cfg.HTTPUser) > 0 && len(cfg.HTTPToken) > 0 && strings.HasPrefix(cfg.RepositoryURL, "http") {
		components := strings.SplitAfter(cfg.RepositoryURL, "://")
		cfg.RepositoryURL = components[0] + cfg.HTTPUser + ":" + cfg.HTTPToken + "@" + components[1]
		log.Infof("updated repo url: %s \n", cfg.RepositoryURL)
	}

	gitCmd, err := git.New(cfg.CloneIntoDir)
	if err != nil {
		return fmt.Errorf("create gitCmd project, error: %v", err)
	}
	checkoutArg := getCheckoutArg(cfg.Commit, cfg.Tag, cfg.Branch)

	originPresent, err := isOriginPresent(gitCmd, cfg.CloneIntoDir, cfg.RepositoryURL)
	if err != nil {
		return fmt.Errorf("check if origin is presented, error: %v", err)
	}

	if originPresent && cfg.ResetRepository {
		if err := resetRepo(gitCmd); err != nil {
			return fmt.Errorf("reset repository, error: %v", err)
		}
	}
	if err := run(gitCmd.Init()); err != nil {
		return fmt.Errorf("init repository, error: %v", err)
	}
	if !originPresent {
		if err := run(gitCmd.RemoteAdd("origin", cfg.RepositoryURL)); err != nil {
			return fmt.Errorf("add remote repository (%s), error: %v", cfg.RepositoryURL, err)
		}
	}

	if !cfg.SSLVerify {
		if err := run(gitCmd.Config("http.sslVerify", "false")); err != nil {
			return fmt.Errorf("set config http.sslVerify false, error: %v", err)
		}
	}

	isPR := cfg.PRRepositoryURL != "" || cfg.PRMergeBranch != "" || cfg.PRID != 0
	if isPR {
		if !cfg.ManualMerge || isPrivate(cfg.PRRepositoryURL) && isFork(cfg.RepositoryURL, cfg.PRRepositoryURL) {
			if err := autoMerge(gitCmd, cfg.PRMergeBranch, cfg.BranchDest, cfg.BuildURL,
				cfg.BuildAPIToken, cfg.CloneDepth, cfg.PRID); err != nil {
				return fmt.Errorf("auto merge, error: %v", err)
			}
		} else {
			if err := manualMerge(gitCmd, cfg.RepositoryURL, cfg.PRRepositoryURL, cfg.Branch,
				cfg.Commit, cfg.BranchDest); err != nil {
				return fmt.Errorf("manual merge, error: %v", err)
			}
		}
	} else if checkoutArg != "" {
		if err := checkout(gitCmd, checkoutArg, cfg.Branch, cfg.CloneDepth, cfg.Tag != ""); err != nil {
			return fmt.Errorf("checkout (%s): %v", checkoutArg, err)
		}
		// Update branch: 'git fetch' followed by a 'git merge' is the same as 'git pull'.
		if checkoutArg == cfg.Branch {
			if err := run(gitCmd.Merge("origin/" + cfg.Branch)); err != nil {
				return fmt.Errorf("merge %q: %v", cfg.Branch, err)
			}
		}
	}

	if cfg.UpdateSubmodules {
		if err := run(gitCmd.SubmoduleUpdate()); err != nil {
			return fmt.Errorf("submodule update: %v", err)
		}
	}

	if isPR {
		if err := run(gitCmd.Checkout("--detach")); err != nil {
			return fmt.Errorf("detach head: %v", err)
		}
	}

	if checkoutArg != "" {
		log.Infof("\nExporting git logs\n")

		for format, env := range map[string]string{
			`%H`:  "GIT_CLONE_COMMIT_HASH",
			`%s`:  "GIT_CLONE_COMMIT_MESSAGE_SUBJECT",
			`%b`:  "GIT_CLONE_COMMIT_MESSAGE_BODY",
			`%an`: "GIT_CLONE_COMMIT_AUTHOR_NAME",
			`%ae`: "GIT_CLONE_COMMIT_AUTHOR_EMAIL",
			`%cn`: "GIT_CLONE_COMMIT_COMMITER_NAME",
			`%ce`: "GIT_CLONE_COMMIT_COMMITER_EMAIL",
		} {
			if err := printLogAndExportEnv(gitCmd, format, env); err != nil {
				return fmt.Errorf("gitCmd log failed, error: %v", err)
			}
		}

		count, err := output(gitCmd.RevList("HEAD", "--count"))
		if err != nil {
			return fmt.Errorf("get rev-list, error: %v", err)
		}

		log.Printf("=> %s\n   value: %s\n", "GIT_CLONE_COMMIT_COUNT", count)
		if err := exportEnvironmentWithEnvman("GIT_CLONE_COMMIT_COUNT", count); err != nil {
			return fmt.Errorf("envman export, error: %v", err)
		}
	}

	return nil
}

func main() {
	if err := mainE(); err != nil {
		log.Errorf("ERROR: %v", err)
		os.Exit(1)
	}
	log.Donef("\nSuccess")
}
