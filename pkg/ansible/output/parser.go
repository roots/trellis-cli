package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// EventHandler processes parsed Ansible JSONL events.
type EventHandler interface {
	OnPlayStart(PlayStartEvent)
	OnTaskStart(TaskStartEvent)
	OnRunnerOk(RunnerOkEvent)
	OnRunnerSkipped(RunnerSkippedEvent)
	OnRunnerFailed(RunnerFailedEvent)
	OnRunnerUnreachable(RunnerUnreachableEvent)
	OnStats(StatsEvent)
	// OnParseError is called when a line cannot be parsed as JSON.
	// The handler may switch to passthrough mode.
	OnParseError(line string, err error)
}

// Parser reads JSONL lines from an io.Reader and dispatches events to a handler.
type Parser struct {
	handler     EventHandler
	passthrough io.Writer
}

// NewParser creates a Parser that dispatches to the given handler.
// The passthrough writer is used when the handler signals a parse error
// and the parser falls back to writing raw lines.
func NewParser(handler EventHandler, passthrough io.Writer) *Parser {
	return &Parser{
		handler:     handler,
		passthrough: passthrough,
	}
}

// Parse reads lines from r and dispatches events until EOF.
func (p *Parser) Parse(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line size for large Gathering Facts events

	passthroughMode := false

	for scanner.Scan() {
		line := scanner.Text()

		if passthroughMode {
			fmt.Fprintln(p.passthrough, line)
			continue
		}

		var envelope EventEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			p.handler.OnParseError(line, err)
			passthroughMode = true
			continue
		}

		if err := p.dispatch(envelope.EventType, []byte(line)); err != nil {
			p.handler.OnParseError(line, err)
			passthroughMode = true
			continue
		}
	}

	return scanner.Err()
}

func (p *Parser) dispatch(eventType string, data []byte) error {
	switch eventType {
	case EventPlayStart:
		var event PlayStartEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("parsing play start: %w", err)
		}
		p.handler.OnPlayStart(event)

	case EventTaskStart:
		var event TaskStartEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("parsing task start: %w", err)
		}
		p.handler.OnTaskStart(event)

	case EventRunnerOk:
		var event RunnerOkEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("parsing runner ok: %w", err)
		}
		p.handler.OnRunnerOk(event)

	case EventRunnerSkipped:
		var event RunnerSkippedEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("parsing runner skipped: %w", err)
		}
		p.handler.OnRunnerSkipped(event)

	case EventRunnerFailed:
		var event RunnerFailedEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("parsing runner failed: %w", err)
		}
		p.handler.OnRunnerFailed(event)

	case EventRunnerUnreachable:
		var event RunnerUnreachableEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("parsing runner unreachable: %w", err)
		}
		p.handler.OnRunnerUnreachable(event)

	case EventStats:
		var event StatsEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("parsing stats: %w", err)
		}
		p.handler.OnStats(event)

	default:
		// Unknown events are silently ignored
	}

	return nil
}
