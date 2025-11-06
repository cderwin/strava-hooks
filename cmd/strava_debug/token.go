package main

import (
	"os"
	"os/exec"
)


func GetToken(AthleteId string, RedisUrl string, StartProxy bool) (string, error) {
	return "", nil
}

func StartProxy() (*os.Process, error) {
	flyExecutable, err := exec.LookPath("fly")
	if err != nil {
		return nil, err
	}

	command := exec.Command(flyExecutable, "redis", "proxy")
	command.Start()
	return command.Process, nil
}
