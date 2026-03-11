package picobota2a

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"
)

// AgentProcessor is the interface that PicobotExecutor needs from the agent loop.
// This avoids importing the agent package directly.
type AgentProcessor interface {
	ProcessDirect(content string, timeout time.Duration) (string, error)
}

// PicobotExecutor implements a2asrv.AgentExecutor, bridging inbound A2A
// requests to the picobot agent loop.
type PicobotExecutor struct {
	agent   AgentProcessor
	timeout time.Duration
}

var _ a2asrv.AgentExecutor = (*PicobotExecutor)(nil)

func NewPicobotExecutor(agent AgentProcessor, timeout time.Duration) *PicobotExecutor {
	return &PicobotExecutor{agent: agent, timeout: timeout}
}

func (e *PicobotExecutor) Execute(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	// Extract text from incoming message parts
	text := extractText(reqCtx.Message)
	if text == "" {
		return e.writeError(ctx, reqCtx, queue, "no text content in message")
	}

	// Signal that we're working
	working := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateWorking, nil)
	if err := queue.Write(ctx, working); err != nil {
		return fmt.Errorf("write working state: %w", err)
	}

	// Process through the agent loop
	result, err := e.agent.ProcessDirect(text, e.timeout)
	if err != nil {
		return e.writeError(ctx, reqCtx, queue, err.Error())
	}

	// Write the response as a completed message
	responseMsg := a2a.NewMessageForTask(a2a.MessageRoleAgent, reqCtx, a2a.TextPart{Text: result})
	completed := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateCompleted, responseMsg)
	completed.Final = true
	if err := queue.Write(ctx, completed); err != nil {
		return fmt.Errorf("write completed state: %w", err)
	}

	return nil
}

func (e *PicobotExecutor) Cancel(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue) error {
	canceled := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateCanceled, nil)
	canceled.Final = true
	return queue.Write(ctx, canceled)
}

func (e *PicobotExecutor) writeError(ctx context.Context, reqCtx *a2asrv.RequestContext, queue eventqueue.Queue, errMsg string) error {
	errMessage := a2a.NewMessageForTask(a2a.MessageRoleAgent, reqCtx, a2a.TextPart{Text: "Error: " + errMsg})
	failed := a2a.NewStatusUpdateEvent(reqCtx, a2a.TaskStateFailed, errMessage)
	failed.Final = true
	return queue.Write(ctx, failed)
}

func extractText(msg *a2a.Message) string {
	if msg == nil {
		return ""
	}
	var texts []string
	for _, p := range msg.Parts {
		if tp, ok := p.(a2a.TextPart); ok {
			texts = append(texts, tp.Text)
		}
	}
	return strings.Join(texts, "\n")
}
