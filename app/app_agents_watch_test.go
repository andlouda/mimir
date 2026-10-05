package main

import (
	"context"
	"testing"
	"time"

	"mimir/agents"
)

func TestAnsweredPromptIsNotReoffered(t *testing.T) {
	a := &App{}
	at := time.Now()
	if a.promptAnswered(1, at) {
		t.Fatal("nothing answered yet")
	}
	a.markPromptAnswered(1, at)
	if !a.promptAnswered(1, at) {
		t.Fatal("the answered prompt must be recognised by its arrival time")
	}
	if a.promptAnswered(1, at.Add(time.Second)) {
		t.Fatal("a newer prompt must not count as answered")
	}
	a.stopAgentWatcher(1)
	if a.promptAnswered(1, at) {
		t.Fatal("stopping the watcher clears the marker")
	}
}

func TestReplacedWatcherReleasesOnlyItself(t *testing.T) {
	a := &App{agentWatchers: map[int]*agentWatcher{}}
	_, cancelOld := context.WithCancel(context.Background())
	_, cancelNew := context.WithCancel(context.Background())
	old := &agentWatcher{cancel: cancelOld, pid: 1}
	a.agentWatchers[7] = old
	// Successor takes over the pane and stores a prompt.
	nw := &agentWatcher{cancel: cancelNew, pid: 2}
	a.agentWatchers[7] = nw
	ev := agents.HookEvent{NotificationType: "permission_prompt", Message: "x"}
	a.setAgentPrompt(7, &ev, time.Now())
	if a.ownsAgentWatcher(7, old) {
		t.Fatal("old watcher must not own the pane")
	}
	a.releaseAgentWatcher(7, old) // the old goroutine exiting
	if a.agentWatchers[7] != nw {
		t.Fatal("old watcher removed the successor's entry")
	}
	if _, held := a.heldPrompt(7); !held {
		t.Fatal("old watcher cleared the successor's prompt")
	}
	a.releaseAgentWatcher(7, nw)
	if _, ok := a.agentWatchers[7]; ok {
		t.Fatal("the current watcher must release its own entry")
	}
}
