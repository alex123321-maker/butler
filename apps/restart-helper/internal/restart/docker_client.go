package restart

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type DockerClient struct {
	httpClient *http.Client
	baseURL    string
}

type dockerContainer struct {
	ID     string            `json:"Id"`
	Labels map[string]string `json:"Labels"`
}

func NewDockerClient(dockerHost string, timeout time.Duration) (*DockerClient, error) {
	trimmed := strings.TrimSpace(dockerHost)
	if trimmed == "" {
		return nil, fmt.Errorf("docker host is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse docker host: %w", err)
	}

	transport := &http.Transport{}
	baseURL := trimmed

	switch parsed.Scheme {
	case "unix":
		socketPath := parsed.Path
		if socketPath == "" {
			socketPath = parsed.Opaque
		}
		if strings.TrimSpace(socketPath) == "" {
			return nil, fmt.Errorf("unix docker host must include socket path")
		}
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		}
		baseURL = "http://docker"
	case "tcp":
		baseURL = "http://" + parsed.Host
	case "http", "https":
		baseURL = strings.TrimRight(trimmed, "/")
	default:
		return nil, fmt.Errorf("unsupported docker host scheme %q", parsed.Scheme)
	}

	return &DockerClient{
		httpClient: &http.Client{Timeout: timeout, Transport: transport},
		baseURL:    baseURL,
	}, nil
}

func (c *DockerClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/_ping", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("docker ping returned %d", resp.StatusCode)
	}
	return nil
}

func (c *DockerClient) RestartService(ctx context.Context, project, service string, timeout time.Duration) error {
	containers, err := c.listServiceContainers(ctx, project, service)
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		return fmt.Errorf("no containers found for project %q service %q", project, service)
	}

	sort.Slice(containers, func(i, j int) bool { return containers[i].ID < containers[j].ID })
	timeoutSeconds := int(timeout / time.Second)
	if timeoutSeconds <= 0 {
		timeoutSeconds = 1
	}

	for _, container := range containers {
		if err := c.restartContainer(ctx, container.ID, timeoutSeconds); err != nil {
			return fmt.Errorf("restart container %s: %w", container.ID, err)
		}
	}
	return nil
}

func (c *DockerClient) listServiceContainers(ctx context.Context, project, service string) ([]dockerContainer, error) {
	filtersJSON, err := json.Marshal(map[string][]string{
		"label": {
			"com.docker.compose.project=" + project,
			"com.docker.compose.service=" + service,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode filters: %w", err)
	}

	query := url.Values{}
	query.Set("all", "1")
	query.Set("filters", string(filtersJSON))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/containers/json?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker list containers returned %d", resp.StatusCode)
	}

	var containers []dockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, fmt.Errorf("decode docker containers: %w", err)
	}
	return containers, nil
}

func (c *DockerClient) restartContainer(ctx context.Context, containerID string, timeoutSeconds int) error {
	query := url.Values{}
	query.Set("t", fmt.Sprintf("%d", timeoutSeconds))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/containers/"+url.PathEscape(containerID)+"/restart?"+query.Encode(), nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("docker restart returned %d", resp.StatusCode)
	}
	return nil
}
