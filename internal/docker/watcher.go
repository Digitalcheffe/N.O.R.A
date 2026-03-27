package docker

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/google/uuid"

	"github.com/digitalcheffe/nora/internal/models"
	"github.com/digitalcheffe/nora/internal/repo"
)

// dockerAPI is the minimal subset of the Docker client we use, enabling mock injection in tests.
type dockerAPI interface {
	Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)
	Ping(ctx context.Context) (types.Ping, error)
	Close() error
}

// Watcher streams container lifecycle events from the Docker daemon and writes
// them to the NORA events table.
type Watcher struct {
	store  *repo.Store
	client dockerAPI
	// onContainerStart is called after a "start" event is processed.
	// It is used to trigger an immediate health check via the HealthPoller.
	onContainerStart func(ctx context.Context, containerID string)
}

// SetContainerStartHook registers a callback that is called (in a goroutine)
// whenever a container start event is received. Used to trigger an immediate
// health check without coupling Watcher and HealthPoller.
func (w *Watcher) SetContainerStartHook(fn func(ctx context.Context, containerID string)) {
	w.onContainerStart = fn
}

// NewWatcher creates a Watcher connected to the Docker daemon. It returns an
// error only for genuine client construction failures. If the socket is simply
// absent the caller should log a warning and skip starting the watcher.
func NewWatcher(store *repo.Store) (*Watcher, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &Watcher{store: store, client: cli}, nil
}

// Start streams Docker container events until ctx is cancelled. On daemon
// disconnect it waits 10 s then reconnects, retrying indefinitely.
func (w *Watcher) Start(ctx context.Context) {
	log.Printf("docker watcher: starting")
	for {
		err := w.stream(ctx)
		if ctx.Err() != nil {
			log.Printf("docker watcher: stopped")
			return
		}
		log.Printf("docker watcher: disconnected (%v) — reconnecting in 10s", err)
		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			log.Printf("docker watcher: stopped")
			return
		}
	}
}

// stream subscribes to the Docker event stream and processes messages until an
// error occurs or ctx is cancelled.
func (w *Watcher) stream(ctx context.Context) error {
	msgCh, errCh := w.client.Events(ctx, events.ListOptions{})
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case msg := <-msgCh:
			if msg.Type != events.ContainerEventType {
				continue
			}
			if err := w.handleEvent(ctx, msg); err != nil {
				log.Printf("docker watcher: handle event: %v", err)
			}
		}
	}
}

// handleEvent maps a Docker container event to a NORA event and persists it.
func (w *Watcher) handleEvent(ctx context.Context, msg events.Message) error {
	action := string(msg.Action)
	switch action {
	case "start", "stop", "die", "restart", "kill":
		// handled below
	default:
		return nil
	}

	containerName := msg.Actor.Attributes["name"]
	exitCodeStr := msg.Actor.Attributes["exitCode"]

	severity, displayText := severityAndText(action, containerName, exitCodeStr)

	// Trigger an immediate health check on container start.
	if action == "start" && w.onContainerStart != nil {
		containerID := msg.Actor.ID
		go w.onContainerStart(ctx, containerID)
	}

	// Try to find a matching app by container name (case-insensitive).
	appID := ""
	apps, err := w.store.Apps.List(ctx)
	if err != nil {
		log.Printf("docker watcher: list apps: %v", err)
	} else {
		for _, a := range apps {
			if strings.EqualFold(a.Name, containerName) {
				appID = a.ID
				break
			}
		}
	}

	fields := fmt.Sprintf(
		`{"source_type":"docker_container","container_name":%s,"action":%s}`,
		jsonStr(containerName), jsonStr(action),
	)
	if exitCodeStr != "" {
		fields = fmt.Sprintf(
			`{"source_type":"docker_container","container_name":%s,"action":%s,"exit_code":%s}`,
			jsonStr(containerName), jsonStr(action), jsonStr(exitCodeStr),
		)
	}

	ev := &models.Event{
		ID:          uuid.New().String(),
		AppID:       appID, // empty string → NULL via NULLIF in repo
		ReceivedAt:  time.Now().UTC(),
		Severity:    severity,
		DisplayText: displayText,
		RawPayload:  "{}",
		Fields:      fields,
	}

	if err := w.store.Events.Create(ctx, ev); err != nil {
		return fmt.Errorf("create event: %w", err)
	}
	return nil
}

// severityAndText returns the severity level and display text for a Docker event.
func severityAndText(action, containerName, exitCodeStr string) (severity, displayText string) {
	switch action {
	case "start":
		return "info", fmt.Sprintf("Container started — %s", containerName)
	case "stop":
		return "warn", fmt.Sprintf("Container stopped — %s", containerName)
	case "die":
		code := parseExitCode(exitCodeStr)
		if code == 0 {
			return "info", fmt.Sprintf("Container exited cleanly — %s", containerName)
		}
		return "error", fmt.Sprintf("Container crashed — %s (exit %d)", containerName, code)
	case "restart":
		return "warn", fmt.Sprintf("Container restarted — %s", containerName)
	case "kill":
		return "warn", fmt.Sprintf("Container killed — %s", containerName)
	default:
		return "info", fmt.Sprintf("Container event %s — %s", action, containerName)
	}
}

func parseExitCode(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// jsonStr returns s as a JSON-encoded string (with quotes and escaping).
func jsonStr(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
