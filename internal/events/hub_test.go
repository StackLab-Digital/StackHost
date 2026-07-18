package events

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestPublishBroadcastsTheSameEvent(t *testing.T) {
	hub := New(2)
	t.Cleanup(hub.Close)
	first := hub.Subscribe()
	second := hub.Subscribe()
	payload := map[string]any{"id": 42, "status": "queued"}

	hub.Publish("deployment.queued", payload)

	gotFirst := receive(t, first)
	gotSecond := receive(t, second)
	if !reflect.DeepEqual(gotFirst, gotSecond) {
		t.Fatalf("subscribers received different events: %#v and %#v", gotFirst, gotSecond)
	}
	if gotFirst.ID != 1 || gotFirst.Event != "deployment.queued" || !reflect.DeepEqual(gotFirst.Data, payload) {
		t.Fatalf("unexpected event: %#v", gotFirst)
	}
	if gotFirst.At.Location() != time.UTC {
		t.Fatalf("event time is not UTC: %v", gotFirst.At)
	}
}

func TestUnsubscribeClosesOnlyThatSubscription(t *testing.T) {
	hub := New(1)
	t.Cleanup(hub.Close)
	removed := hub.Subscribe()
	active := hub.Subscribe()

	hub.Unsubscribe(removed)
	hub.Unsubscribe(removed)
	if _, open := <-removed; open {
		t.Fatal("unsubscribed stream is still open")
	}

	hub.Publish("application.updated", map[string]any{"id": 7})
	if got := receive(t, active); got.ID != 1 || got.Event != "application.updated" {
		t.Fatalf("active subscription received %#v", got)
	}
}

func TestConcurrentPublishPreservesBroadcastOrder(t *testing.T) {
	const publications = 64
	hub := New(publications)
	t.Cleanup(hub.Close)
	first := hub.Subscribe()
	second := hub.Subscribe()
	start := make(chan struct{})
	var publishers sync.WaitGroup

	for i := 0; i < publications; i++ {
		publishers.Add(1)
		go func(value int) {
			defer publishers.Done()
			<-start
			hub.Publish("deployment.progress", value)
		}(i)
	}
	close(start)
	publishers.Wait()

	for id := uint64(1); id <= publications; id++ {
		gotFirst := receive(t, first)
		gotSecond := receive(t, second)
		if gotFirst.ID != id || gotSecond.ID != id {
			t.Fatalf("event order at position %d: first=%d second=%d", id, gotFirst.ID, gotSecond.ID)
		}
		if !reflect.DeepEqual(gotFirst, gotSecond) {
			t.Fatalf("event %d differs between subscriptions: %#v and %#v", id, gotFirst, gotSecond)
		}
	}
}

func TestSlowSubscriberDoesNotBlockOthers(t *testing.T) {
	hub := New(1)
	t.Cleanup(hub.Close)
	slow := hub.Subscribe()
	hub.Publish("first", nil)
	fast := hub.Subscribe()

	done := make(chan struct{})
	go func() {
		hub.Publish("second", nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publish blocked on a full subscriber buffer")
	}

	if got := receive(t, fast); got.ID != 2 || got.Event != "second" {
		t.Fatalf("fast subscriber received %#v", got)
	}
	if got := receive(t, slow); got.ID != 1 || got.Event != "first" {
		t.Fatalf("slow subscriber lost its buffered event: %#v", got)
	}
	select {
	case got := <-slow:
		t.Fatalf("full subscriber should have dropped the new event, got %#v", got)
	default:
	}
}

func TestCloseIsIdempotentAndClosesFutureSubscriptions(t *testing.T) {
	hub := New(1)
	subscriber := hub.Subscribe()

	hub.Close()
	hub.Close()
	hub.Publish("ignored", nil)
	if _, open := <-subscriber; open {
		t.Fatal("subscriber is still open after close")
	}
	if _, open := <-hub.Subscribe(); open {
		t.Fatal("subscription created after close is open")
	}
}

func TestPublisherFuncAdaptsCallback(t *testing.T) {
	var gotEvent string
	var gotData any
	var publisher Publisher = PublisherFunc(func(event string, data any) {
		gotEvent, gotData = event, data
	})

	publisher.Publish("deployment.failed", 9)

	if gotEvent != "deployment.failed" || gotData != 9 {
		t.Fatalf("callback received event=%q data=%#v", gotEvent, gotData)
	}
}

func receive(t *testing.T, subscriber <-chan Event) Event {
	t.Helper()
	select {
	case event, open := <-subscriber:
		if !open {
			t.Fatal("subscription closed before an event was received")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}
