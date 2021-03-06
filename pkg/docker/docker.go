package docker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	dockerConfig "github.com/docker/cli/cli/config"
	configTypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/kyma-project/cli/internal/minikube"
	"github.com/kyma-project/cli/pkg/step"
)

const (
	defaultRegistry = "index.docker.io"
)

type dockerClient struct {
	*docker.Client
}

type kymaDockerClient struct {
	Docker Client
}

//go:generate mockery --name Client
type Client interface {
	ArchiveDirectory(srcPath string, options *archive.TarOptions) (io.ReadCloser, error)
	NegotiateAPIVersion(ctx context.Context)
	ImagePush(ctx context.Context, image string, options types.ImagePushOptions) (io.ReadCloser, error)
	ImageBuild(ctx context.Context, buildContext io.Reader, options types.ImageBuildOptions) (types.ImageBuildResponse, error)
}

type KymaClient interface {
	PushKymaInstaller(image string, currentStep step.Step) error
	BuildKymaInstaller(localSrcPath, imageName string) error
}

// ErrorMessage is used to parse error messages coming from Docker
type ErrorMessage struct {
	Error string
}

//NewDockerService creates docker client using docker environment of the OS
func NewClient() (Client, error) {
	dClient, err := docker.NewClientWithOpts(docker.FromEnv)
	if err != nil {
		return nil, err
	}

	return &dockerClient{
		dClient,
	}, nil
}

//NewDockerMinikubeService creates docker client for minikube docker-env
func NewMinikubeClient(verbosity bool, profile string, timeout time.Duration) (Client, error) {
	dClient, err := minikube.DockerClient(verbosity, profile, timeout)
	if err != nil {
		return nil, err
	}

	return &dockerClient{
		dClient,
	}, nil
}

func NewKymaClient(isLocal bool, verbosity bool, profile string, timeout time.Duration) (KymaClient, error) {
	var err error
	var dc Client
	if isLocal {
		dc, err = NewMinikubeClient(verbosity, profile, timeout)
	} else {
		dc, err = NewClient()
	}
	return &kymaDockerClient{
		Docker: dc,
	}, err
}

func (d *dockerClient) ArchiveDirectory(srcPath string, options *archive.TarOptions) (io.ReadCloser, error) {
	return archive.TarWithOptions(srcPath, &archive.TarOptions{})
}

func (k *kymaDockerClient) BuildKymaInstaller(localSrcPath, imageName string) error {
	reader, err := k.Docker.ArchiveDirectory(localSrcPath, &archive.TarOptions{})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(300)*time.Second)
	defer cancel()
	k.Docker.NegotiateAPIVersion(ctx)
	args := make(map[string]*string)
	_, err = k.Docker.ImageBuild(
		ctx,
		reader,
		types.ImageBuildOptions{
			Tags:           []string{strings.TrimSpace(string(imageName))},
			SuppressOutput: true,
			Remove:         true,
			Dockerfile:     path.Join("tools", "kyma-installer", "kyma.Dockerfile"),
			BuildArgs:      args,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func (k *kymaDockerClient) PushKymaInstaller(image string, currentStep step.Step) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(300)*time.Second)
	defer cancel()
	k.Docker.NegotiateAPIVersion(ctx)
	domain, _ := splitDockerDomain(image)
	auth, err := resolve(domain)
	if err != nil {
		return err
	}

	encodedJSON, err := json.Marshal(auth)
	if err != nil {
		return err
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)

	currentStep.LogInfof("Pushing Docker image: '%s'", image)

	pusher, err := k.Docker.ImagePush(ctx, image, types.ImagePushOptions{RegistryAuth: authStr})
	if err != nil {
		return err
	}

	defer pusher.Close()

	var errorMessage ErrorMessage
	buffIOReader := bufio.NewReader(pusher)

	for {
		streamBytes, err := buffIOReader.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		err = json.Unmarshal(streamBytes, &errorMessage)
		if err != nil {
			return err
		}
		if errorMessage.Error != "" {
			if strings.Contains(errorMessage.Error, "unauthorized") || strings.Contains(errorMessage.Error, "requested access to the resource is denied") {
				return fmt.Errorf("missing permissions to push Docker image: %s\nPlease run `docker login` to authenticate", errorMessage.Error)
			}
			return fmt.Errorf("failed to push Docker image: %s", errorMessage.Error)
		}
	}

	return nil
}

func splitDockerDomain(name string) (domain, remainder string) {
	i := strings.IndexRune(name, '/')
	if i == -1 || (!strings.ContainsAny(name[:i], ".:") && name[:i] != "localhost") {
		domain, remainder = defaultRegistry, name
	} else {
		domain, remainder = name[:i], name[i+1:]
	}
	return
}

// resolve finds the Docker credentials to push an image.
func resolve(registry string) (*configTypes.AuthConfig, error) {
	cf, err := dockerConfig.Load(os.Getenv("DOCKER_CONFIG"))
	if err != nil {
		return nil, err
	}

	if registry == defaultRegistry {
		registry = "https://" + defaultRegistry + "/v1/"
	}

	cfg, err := cf.GetAuthConfig(registry)
	if err != nil {
		return nil, err
	}

	empty := configTypes.AuthConfig{}
	if cfg == empty {
		return &empty, nil
	}
	return &configTypes.AuthConfig{
		Username:      cfg.Username,
		Password:      cfg.Password,
		Auth:          cfg.Auth,
		IdentityToken: cfg.IdentityToken,
		RegistryToken: cfg.RegistryToken,
	}, nil
}
