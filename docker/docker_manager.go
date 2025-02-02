package docker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"go.viam.com/rdk/logging"
)

var ErrImageDoesNotExist = errors.New("image does not exist")

type DockerManager interface {
	ListContainers() ([]DockerContainerDetails, error)
	ListImages() ([]DockerImageDetails, error)
	GetImageDetails(imageId string) (DockerImageDetails, error)
	InspectContainer(containerHash string) (map[string]interface{}, error)
	GetContainerImageId(containerId string) (string, error)
	GetContainerImageDigest(containerId string) (string, error)
	GetContainersRunningImage(imageDigest string) ([]DockerContainerDetails, error)
	PullImage(name string, repoDigest string) error
	PullPrivateImage(name string, githubUsername string, pat string) error
	RemoveImageByImageId(imageId string) error
	RemoveImageByRepoDigest(repoDigest string) error
}

type LocalDockerManager struct {
	logger logging.Logger
}

type DockerImageDetails struct {
	Repository string
	Tag        string
	RepoDigest string
	ImageID    string
	Created    string
	Size       string
}

type DockerContainerDetails struct {
	ContainerID string
	Image       string
	Command     string
	Created     string
	Status      string
	Ports       string
	Names       string
}

func NewLocalDockerManager(logger logging.Logger) DockerManager {
	return &LocalDockerManager{logger: logger}
}

func (dm *LocalDockerManager) ListImages() ([]DockerImageDetails, error) {
	proc := exec.Command("docker", "images", "--digests", "--no-trunc")
	outputBytes, err := proc.Output()
	if err != nil {
		return nil, err
	}

	var containers []DockerImageDetails

	scanner := bufio.NewScanner(strings.NewReader(string(outputBytes)))

	// Skipt the first line
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()
		//REPOSITORY   TAG       DIGEST                                                                    IMAGE ID                                                                  CREATED       SIZE
		//ubuntu       latest    sha256:2b7412e6465c3c7fc5bb21d3e6f1917c167358449fecac8176c6e496e5c1f05f   sha256:e4c58958181a5925816faa528ce959e487632f4cfd192f8132f71b32df2744b4   4 weeks ago   77.8MB
		//ubuntu       <none>    sha256:218bb51abbd1864df8be26166f847547b3851a89999ca7bfceb85ca9b5d2e95d   sha256:bf40b7bc7a11b43785755d3c5f23dee03b08e988b327a2f10b22d01d5dc5259d   4 weeks ago   72.8MB

		// Extract fields from the line
		tmpParts := strings.Split(line, "  ")
		var parts []string
		for _, part := range tmpParts {
			if part != "" {
				parts = append(parts, strings.TrimSpace(part))
			}
		}

		// Make sure there are enough parts to actually parse without erroring
		if len(parts) < 6 {
			continue
		}

		// Add to containers slice
		containers = append(containers, DockerImageDetails{
			Repository: parts[0],
			Tag:        parts[1],
			RepoDigest: parts[2],
			ImageID:    parts[3],
			Created:    parts[4],
			Size:       parts[5],
		})
	}

	return containers, nil
}

func (dm *LocalDockerManager) RemoveImageByImageId(imageId string) error {
	proc := exec.Command("docker", "rmi", imageId)
	_, err := proc.Output()
	if err != nil {
		return err
	}

	return nil
}

func (dm *LocalDockerManager) RemoveImageByRepoDigest(repoDigest string) error {
	images, err := dm.ListImages()
	if err != nil {
		return err
	}
	var imageId string
	for _, image := range images {
		if image.RepoDigest == repoDigest {
			imageId = image.ImageID
		}
	}
	proc := exec.Command("docker", "rmi", imageId)
	_, err = proc.Output()
	if err != nil {
		return err
	}

	return nil
}

func (dm *LocalDockerManager) ListContainers() ([]DockerContainerDetails, error) {
	proc := exec.Command("docker", "container", "ls", "--all", "--no-trunc")
	outputBytes, err := proc.Output()
	if err != nil {
		return nil, err
	}

	var containers []DockerContainerDetails

	scanner := bufio.NewScanner(strings.NewReader(string(outputBytes)))

	// Skip the first line
	scanner.Scan()

	// Let's pull out the width of each column
	t := scanner.Text()
	containerIdLen := strings.Index(t, "IMAGE")
	if containerIdLen == -1 {
		return nil, errors.New("failed to parse output")
	}
	imageLen := strings.Index(t, "COMMAND")
	if imageLen == -1 {
		return nil, errors.New("failed to parse output")
	}

	commandLen := strings.Index(t, "CREATED")
	if commandLen == -1 {
		return nil, errors.New("failed to parse output")
	}

	createdLen := strings.Index(t, "STATUS")
	if createdLen == -1 {
		return nil, errors.New("failed to parse output")
	}

	statusLen := strings.Index(t, "PORTS")
	if statusLen == -1 {
		return nil, errors.New("failed to parse output")
	}

	portsLen := strings.Index(t, "NAMES")
	if portsLen == -1 {
		return nil, errors.New("failed to parse output")
	}

	for scanner.Scan() {
		line := scanner.Text()
		// CONTAINER ID                                                       IMAGE                                                                            COMMAND   CREATED          STATUS          PORTS     NAMES
		// a6269652d8c38a31ed1256f81970d5070fd1d9050a8ac6304f255f05b4ed1b76   ubuntu:latest                                                                    "bash"    5 seconds ago    Up 3 seconds              eager_pike
		// 5e57ddd38731cb96bd71da445c3fcfd952d5863c90ff9db4eefb335834308097   ubuntu@sha256:218bb51abbd1864df8be26166f847547b3851a89999ca7bfceb85ca9b5d2e95d   "bash"    11 minutes ago   Up 11 minutes             pensive_ishizaka

		// Extract fields from the line

		// Add to containers slice
		containers = append(containers, DockerContainerDetails{
			ContainerID: strings.TrimSpace(line[:containerIdLen]),
			Image:       strings.TrimSpace(line[containerIdLen:imageLen]),
			Command:     strings.TrimSpace(line[imageLen:commandLen]),
			Created:     strings.TrimSpace(line[commandLen:createdLen]),
			Status:      strings.TrimSpace(line[createdLen:statusLen]),
			Ports:       strings.TrimSpace(line[statusLen:portsLen]),
			Names:       strings.TrimSpace(line[portsLen:]),
		})
	}

	return containers, nil
}

func (dm *LocalDockerManager) GetImageDetails(imageId string) (DockerImageDetails, error) {
	images, err := dm.ListImages()
	if err != nil {
		return DockerImageDetails{}, err
	}

	for _, image := range images {
		if image.ImageID == imageId {
			return image, nil
		}
	}
	return DockerImageDetails{}, errors.New("image not found")
}

func (dm *LocalDockerManager) InspectContainer(containerHash string) (map[string]interface{}, error) {
	proc := exec.Command("docker", "container", "inspect", containerHash)
	outputBytes, err := proc.Output()
	if err != nil {
		return nil, err
	}

	var container []map[string]interface{}

	err = json.Unmarshal(outputBytes, &container)
	if err != nil {
		return nil, err
	}

	return container[0], nil
}

func (dm *LocalDockerManager) GetContainerImageId(containerId string) (string, error) {
	container, err := dm.InspectContainer(containerId)
	if err != nil {
		return "", err
	}

	return container["Image"].(string), nil
}

func (dm *LocalDockerManager) GetContainerImageDigest(containerId string) (string, error) {
	imageId, err := dm.GetContainerImageId(containerId)
	if err != nil {
		return "", err
	}

	image, err := dm.GetImageDetails(imageId)
	if err != nil {
		return "", err
	}

	return image.RepoDigest, nil
}

func (dm *LocalDockerManager) GetContainersRunningImage(repoDigest string) ([]DockerContainerDetails, error) {
	containers, err := dm.ListContainers()
	if err != nil {
		return nil, err
	}

	var containersRunningImage []DockerContainerDetails
	for _, container := range containers {
		digest, err := dm.GetContainerImageDigest(container.ContainerID)
		if err != nil {
			continue
		}
		if digest == repoDigest {
			containersRunningImage = append(containersRunningImage, container)
		}
	}

	return containersRunningImage, nil
}

func (dm *LocalDockerManager) PullImage(name string, repoDigest string) error {
	dm.logger.Debugf("Pulling image %s %s", name, repoDigest)
	proc := exec.Command("docker", "pull", fmt.Sprintf("%s@%s", name, repoDigest))
	// TODO: Read output from proc using a pipe
	// output:=proc.StdoutPipe()

	outputBytes, err := proc.Output()
	if err != nil {
		exitError := err.(*exec.ExitError)
		if exitError != nil && exitError.Stderr != nil {
			dm.logger.Errorf("Output: %s", string(exitError.Stderr))
		}
		dm.logger.Error(err)
		return err
	}
	dm.logger.Debugf("Output: %s", string(outputBytes))
	return nil
}

func (dm *LocalDockerManager) PullPrivateImage(name string, githubUsername string, pat string) error {
	dm.logger.Debugf("Authenticating with GitHub Container Registry")

	// Using exec.Command for login
	loginCmd := exec.Command("docker", "login", "ghcr.io", "-u", githubUsername, "--password-stdin")
	stdin, err := loginCmd.StdinPipe()
	if err != nil {
		dm.logger.Errorf("Failed to create stdin pipe for login command: %v", err)
		return err
	}

	// Capture both standard output and standard error
	var stdout, stderr bytes.Buffer
	loginCmd.Stdout = &stdout
	loginCmd.Stderr = &stderr

	// Start the login command
	if err := loginCmd.Start(); err != nil {
		dm.logger.Errorf("Failed to start login command: %v", err)
		return err
	}

	// Write the PAT to the stdin of the login command
	_, err = stdin.Write([]byte(pat + "\n"))
	if err != nil {
		dm.logger.Errorf("Failed to write PAT to login command: %v", err)
		return err
	}
	stdin.Close()

	// Wait for the login command to complete
	err = loginCmd.Wait()
	if err != nil {
		dm.logger.Errorf("Login command failed: %v", err)
		return err
	}

	// Optionally check the stdout or stderr for specific success/error messages
	loginOutput := stdout.String()
	if strings.Contains(loginOutput, "Login Succeeded") {
		dm.logger.Warnf("Successfully authenticated with GitHub Container Registry")
	} else {
		dm.logger.Errorf("Authentication may have failed, output: %s", loginOutput)
		// You may choose to return an error here based on your requirements
	}

	// Pull command
	dm.logger.Debugf("Pulling image %s", name)
	pullCmd := exec.Command("docker", "pull", name)

	outputBytes, err := pullCmd.CombinedOutput()
	if err != nil {
		dm.logger.Errorf("Failed to pull image: %v. Output: %s", err, string(outputBytes))
		return err
	}

	dm.logger.Debugf("Output: %s", string(outputBytes))
	return nil
}

