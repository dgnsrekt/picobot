package a2abridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
	"github.com/local/picobot/internal/chat"
)

// identityFile is a minimal subset of workspace/identity.json used to populate
// the A2A agent card. Kept local to avoid a cross-package dependency.
type identityFile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Skills      []struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	} `json:"skills"`
}

// A2AServer exposes picobot as an inbound A2A HTTP endpoint.
// Incoming tasks are pushed into the agent hub and the response is collected
// from the hub's outbound channel for the "a2a" subscriber.
type A2AServer struct {
	hub       *chat.Hub
	workspace string
	pending   sync.Map        // key: task ID string → chan string (buffered 1)
	outCh     <-chan chat.Outbound
}

var _ a2asrv.AgentExecutor = (*A2AServer)(nil)

// NewServer creates an A2AServer and subscribes to the "a2a" outbound channel.
// workspace is the path to the agent's workspace directory (used to read
// identity.json per request for the agent card endpoint).
// This must be called BEFORE hub.StartRouter so the subscription is registered
// in time.
func NewServer(hub *chat.Hub, workspace string) *A2AServer {
	outCh := hub.Subscribe("a2a")
	return &A2AServer{hub: hub, workspace: workspace, outCh: outCh}
}

// readIdentity reads workspace/identity.json and returns its contents.
// Returns a zero-value struct on any error so callers always get usable data.
func (s *A2AServer) readIdentity() identityFile {
	var id identityFile
	raw, err := os.ReadFile(filepath.Join(s.workspace, "identity.json"))
	if err != nil {
		return id
	}
	_ = json.Unmarshal(raw, &id)
	return id
}

// buildAgentCard constructs an a2a.AgentCard from identity.json, falling back
// to generic defaults for any missing fields.
func (s *A2AServer) buildAgentCard(port int) *a2a.AgentCard {
	id := s.readIdentity()

	name := id.Name
	if name == "" {
		name = "picobot"
	}
	desc := id.Description
	if desc == "" {
		desc = "Picobot A2A inbound agent"
	}
	cardURL := id.URL
	if cardURL == "" {
		cardURL = fmt.Sprintf("http://0.0.0.0:%d", port)
	}

	skills := make([]a2a.AgentSkill, 0, len(id.Skills)+1)
	for _, s := range id.Skills {
		skills = append(skills, a2a.AgentSkill{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			Tags:        s.Tags,
		})
	}
	if len(skills) == 0 {
		skills = []a2a.AgentSkill{{
			ID:          "chat",
			Name:        "Chat",
			Description: "Send a message to the agent and receive a response.",
			Tags:        []string{"chat"},
		}}
	}

	return &a2a.AgentCard{
		Name:               name,
		Description:        desc,
		URL:                cardURL,
		PreferredTransport: a2a.TransportProtocolJSONRPC,
		ProtocolVersion:    string(a2a.Version),
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Skills:             skills,
	}
}

// Execute implements a2asrv.AgentExecutor. It routes the incoming A2A task
// through the hub, waits for the agent's response, and writes A2A events to
// the queue.
func (s *A2AServer) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	if reqCtx.Message == nil {
		return s.failTask(ctx, reqCtx, queue, "no message in request")
	}
	text := extractPartsText(reqCtx.Message.Parts)
	if text == "" {
		return s.failTask(ctx, reqCtx, queue, "empty message text")
	}

	taskID := string(reqCtx.TaskID)

	// Register a buffered response channel before pushing to the hub so
	// routeOutbound can deliver the reply even if it arrives very quickly.
	respCh := make(chan string, 1)
	s.pending.Store(taskID, respCh)
	defer s.pending.Delete(taskID)

	// Notify the client that work has started.
	workingEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateWorking, nil)
	if err := queue.Write(ctx, workingEvent); err != nil {
		return fmt.Errorf("failed to write working status: %w", err)
	}

	// Push the inbound message into the agent loop.
	select {
	case s.hub.In <- chat.Inbound{
		Channel:  "a2a",
		SenderID: "a2a",
		ChatID:   taskID,
		Content:  text,
	}:
	case <-ctx.Done():
		return nil
	}

	// Wait for the agent's response, a timeout, or context cancellation.
	select {
	case response, ok := <-respCh:
		if !ok {
			// Channel was closed (e.g. by Cancel).
			return s.failTask(ctx, reqCtx, queue, "task was canceled before a response was produced")
		}
		artifactEvent := a2a.NewArtifactEvent(reqCtx, a2a.TextPart{Text: response})
		if err := queue.Write(ctx, artifactEvent); err != nil {
			return fmt.Errorf("failed to write artifact: %w", err)
		}
		completedEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateCompleted, nil)
		completedEvent.Final = true
		if err := queue.Write(ctx, completedEvent); err != nil {
			return fmt.Errorf("failed to write completed status: %w", err)
		}
		return nil

	case <-time.After(90 * time.Second):
		return s.failTask(ctx, reqCtx, queue, "timeout waiting for agent response")

	case <-ctx.Done():
		// Context was canceled (e.g. the client sent a cancel request which wrote
		// a Canceled event; the framework then cancels this context).
		return nil
	}
}

// Cancel implements a2asrv.AgentExecutor. It writes a Canceled terminal event
// so the framework cancels the active Execute context.
func (s *A2AServer) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	canceledEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateCanceled, nil)
	canceledEvent.Final = true
	return queue.Write(ctx, canceledEvent)
}

// failTask writes a Failed terminal event with a human-readable reason and
// returns nil so Execute does not double-report an error to the framework.
func (s *A2AServer) failTask(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue, reason string) error {
	failMsg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.TextPart{Text: reason})
	failEvent := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateFailed, failMsg)
	failEvent.Final = true
	if err := queue.Write(ctx, failEvent); err != nil {
		return fmt.Errorf("failed to write failed status: %w", err)
	}
	return nil
}

// routeOutbound reads from the "a2a" outbound channel and delivers each reply
// to the waiting Execute goroutine via its pending channel.
func (s *A2AServer) routeOutbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case out, ok := <-s.outCh:
			if !ok {
				return
			}
			if ch, loaded := s.pending.Load(out.ChatID); loaded {
				select {
				case ch.(chan string) <- out.Content:
				default:
					// Channel already has a value; discard duplicate.
				}
			}
		}
	}
}

// Start launches the routeOutbound goroutine and begins serving A2A JSON-RPC
// requests on the given port. It blocks until ctx is canceled.
func (s *A2AServer) Start(ctx context.Context, port int) error {
	go s.routeOutbound(ctx)

	requestHandler := a2asrv.NewHandler(s)

	// Serve the agent card dynamically so identity.json changes take effect
	// without a restart.
	cardHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		card := s.buildAgentCard(port)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(card)
	})

	mux := http.NewServeMux()
	mux.Handle(a2asrv.WellKnownAgentCardPath, cardHandler)
	mux.Handle("/", a2asrv.NewJSONRPCHandler(requestHandler))

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("a2a: failed to listen on :%d: %w", port, err)
	}

	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	log.Printf("a2a: inbound server listening on :%d", port)
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
