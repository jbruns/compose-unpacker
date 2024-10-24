package main

import (
	"path"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"
)

func deploySwarmStack(cmd SwarmDeployCommand, clonePath string) error {
	command := getDockerBinaryPath()
	args := []string{"--config", PORTAINER_DOCKER_CONFIG_PATH}

	if cmd.Prune {
		args = append(args, "stack", "deploy", "--prune", "--with-registry-auth")
	} else {
		args = append(args, "stack", "deploy", "--with-registry-auth")
	}

	if !cmd.Pull {
		args = append(args, "--resolve-image=never")
	}

	for _, cfile := range cmd.ComposeRelativeFilePaths {
		args = append(args, "--compose-file", path.Join(clonePath, cfile))
	}
	log.Info().
		Strs("composeFilePaths", cmd.ComposeRelativeFilePaths).
		Str("workingDirectory", clonePath).
		Str("projectName", cmd.ProjectName).
		Msg("Deploying Swarm stack")

	args = append(args, cmd.ProjectName)

	err := runCommandAndCaptureStdErr(command, args, cmd.Env, clonePath)
	if err != nil {
		log.Error().
			Err(err).
			Msg("Failed to swarm deploy Git repository")
		return errDeployComposeFailure
	}
	log.Info().
		Msg("Swarm stack deployment complete")

	return err
}

func checkRunningService(projectName string) ([]string, error) {
	command := getDockerBinaryPath()
	args := []string{"--config", PORTAINER_DOCKER_CONFIG_PATH, "stack", "services", "--format={{.ID}}", projectName}

	log.Info().
		Strs("args", args).
		Msg("Checking Swarm stack")

	output, err := runCommand(command, args)
	if err != nil {
		log.Error().
			Err(err).
			Msg("Failed to check running swarm services")
		return nil, err
	}

	serviceIDs := splitLines(string(output))
	log.Info().
		Strs("serviceIDs", serviceIDs).
		Msg("Checking stack services")
	return serviceIDs, nil
}

func updateService(serviceID string, forceRecreate bool) error {
	command := getDockerBinaryPath()
	args := []string{"--config", PORTAINER_DOCKER_CONFIG_PATH, "service", "update", serviceID}
	if forceRecreate {
		args = append(args, "--force")
	}

	log.Info().
		Strs("args", args).
		Msg("Updating Swarm service")

	out, err := runCommand(command, args)
	if err != nil {
		log.Error().
			Err(err).
			Str("standard_output", out).
			Str("context", "SwarmDeployerUpdateService").
			Msg("Failed to update swarm services")
		return err
	}

	log.Info().
		Str("standard_output", out).
		Str("context", "SwarmDeployerUpdateService").
		Msg("Update stack service completed")
	return nil
}

func splitLines(s string) []string {
	var separator string
	if runtime.GOOS == "windows" {
		separator = "\r\n"
	} else {
		separator = "\n"
	}
	parts := strings.Split(s, separator)

	ret := []string{}
	for _, part := range parts {
		// remove empty string
		if part != "" {
			ret = append(ret, part)
		}
	}
	return ret
}
