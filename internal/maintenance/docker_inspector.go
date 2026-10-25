package maintenance

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
)

// DockerInspector is fixed private node configuration. It has no write API,
// general URL, author-selected target, signing endpoint or model capability.
type DockerInspector struct {
	client    *http.Client
	container string
}

type PhysicalIdentity struct {
	State          string    `json:"state"`
	ContainerID    string    `json:"container_id"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	RuntimeSHA256  string    `json:"runtime_sha256"`
	MeasuredAt     time.Time `json:"measured_at"`
}

var maintenanceContainerName = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,62}$`)

// NewDockerInspector accepts only a local Unix socket and one literal operator
// target. Core/author processes must never receive this socket. No proxy,
// redirect, daemon discovery, process launch or remote daemon is supported.
func NewDockerInspector(socket, container string) (*DockerInspector, error) {
	if !filepath.IsAbs(socket) || !maintenanceContainerName.MatchString(container) {
		return nil, ErrState
	}
	info, err := os.Lstat(socket)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return nil, ErrState
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 3 * time.Second, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", socket)
	}}
	return &DockerInspector{container: container, client: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrState }}}, nil
}

// Inspect reports actual immutable Docker image identity and measured runtime
// configuration. Running is not functional success or proof of a settled effect.
// It cannot populate LastAttemptID/EffectSettled or clear UNKNOWN on its own.
func (inspector *DockerInspector) Inspect(ctx context.Context) (PhysicalIdentity, error) {
	if inspector == nil || inspector.client == nil || !maintenanceContainerName.MatchString(inspector.container) {
		return PhysicalIdentity{}, ErrState
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+inspector.container+"/json", nil)
	if err != nil {
		return PhysicalIdentity{}, ErrState
	}
	response, err := inspector.client.Do(request)
	if err != nil {
		return PhysicalIdentity{}, ErrState
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return PhysicalIdentity{State: "missing", MeasuredAt: time.Now().UTC().Truncate(time.Microsecond)}, nil
	}
	if response.StatusCode != http.StatusOK {
		return PhysicalIdentity{}, ErrState
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 256*1024+1))
	if err != nil || len(raw) > 256*1024 {
		return PhysicalIdentity{}, ErrState
	}
	return inspectDockerIdentity(raw, inspector.container, time.Now())
}

func inspectDockerIdentity(raw []byte, container string, now time.Time) (PhysicalIdentity, error) {
	// Reuse the existing bounded, duplicate-rejecting canonical JSON contract.
	// Raw Docker configuration (including any environment) stays inside the node.
	if _, err := actionauth.ArgumentsHash(string(raw)); err != nil {
		return PhysicalIdentity{}, ErrState
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil || root == nil || dockerFieldAliases(root, "Id", "Name", "Image", "State", "Config", "HostConfig", "Mounts", "NetworkSettings") {
		return PhysicalIdentity{}, ErrState
	}
	var id, name, image string
	var state struct {
		Running    bool
		Paused     bool
		Restarting bool
		Dead       bool
		Status     string
	}
	var stateFields map[string]json.RawMessage
	if json.Unmarshal(root["State"], &stateFields) != nil || dockerFieldAliases(stateFields, "Running", "Paused", "Restarting", "Dead", "Status") {
		return PhysicalIdentity{}, ErrState
	}
	for _, key := range []string{"Running", "Paused", "Restarting", "Dead"} {
		var flag *bool
		if json.Unmarshal(stateFields[key], &flag) != nil || flag == nil {
			return PhysicalIdentity{}, ErrState
		}
	}
	if json.Unmarshal(root["Id"], &id) != nil || !validDigest(id) || json.Unmarshal(root["Name"], &name) != nil || name != "/"+container || json.Unmarshal(root["Image"], &image) != nil || len(image) != 71 || image[:7] != "sha256:" || !validDigest(image[7:]) || json.Unmarshal(root["State"], &state) != nil || state.Paused || state.Restarting || state.Dead {
		return PhysicalIdentity{}, ErrState
	}
	status := "stopped"
	if state.Running {
		if state.Status != "running" {
			return PhysicalIdentity{}, ErrState
		}
		status = "running"
	} else if state.Status != "created" && state.Status != "exited" {
		return PhysicalIdentity{}, ErrState
	}
	var config, host map[string]json.RawMessage
	var mounts []json.RawMessage
	var networkSettings map[string]json.RawMessage
	var attached map[string]map[string]json.RawMessage
	if json.Unmarshal(root["Config"], &config) != nil || config == nil || json.Unmarshal(root["HostConfig"], &host) != nil || host == nil || json.Unmarshal(root["Mounts"], &mounts) != nil || mounts == nil || json.Unmarshal(root["NetworkSettings"], &networkSettings) != nil || dockerFieldAliases(networkSettings, "Networks") || json.Unmarshal(networkSettings["Networks"], &attached) != nil || attached == nil {
		return PhysicalIdentity{}, ErrState
	}
	// Image content is bound separately. Keep every other Config/HostConfig field,
	// mounts and actual network membership; do not trust a label claiming a digest.
	delete(config, "Image")
	networks := map[string]string{}
	for name, network := range attached {
		var networkID string
		if name == "" || dockerFieldAliases(network, "NetworkID") || json.Unmarshal(network["NetworkID"], &networkID) != nil || !validDigest(networkID) {
			return PhysicalIdentity{}, ErrState
		}
		networks[name] = networkID
	}
	runtime := map[string]any{"version": "butler.maintenance.docker-runtime.v1", "config": config, "host_config": host, "mounts": mounts, "networks": networks}
	encoded, err := json.Marshal(runtime)
	if err != nil {
		return PhysicalIdentity{}, ErrState
	}
	digest, err := actionauth.ArgumentsHash(string(encoded))
	if err != nil {
		return PhysicalIdentity{}, ErrState
	}
	return PhysicalIdentity{State: status, ContainerID: id, ArtifactSHA256: image[7:], RuntimeSHA256: digest, MeasuredAt: now.UTC().Truncate(time.Microsecond)}, nil
}

func dockerFieldAliases(object map[string]json.RawMessage, known ...string) bool {
	for key := range object {
		for _, field := range known {
			if strings.EqualFold(key, field) && key != field {
				return true
			}
		}
	}
	return false
}
