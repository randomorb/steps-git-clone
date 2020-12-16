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

func main() {
	var cfg gitclone.Config
	if err := stepconf.Parse(&cfg); err != nil {
		failf("Error: %s\n", err)
	}
	stepconf.Print(cfg)

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

	if err := gitclone.Execute(cfg); err != nil {
		failf("ERROR: %v", err)
	}
	log.Donef("\nSuccess")
}
