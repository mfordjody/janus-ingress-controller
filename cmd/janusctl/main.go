package main

import (
	"janus-ingress-controller/cmd"
)

func main() {
	rootCmd := cmd.GetRootCmd()
	if err := rootCmd.Execute(); err != nil {
		getExitCode(err)
	}
}

func getExitCode(e error) string {
	return e.Error()
}
