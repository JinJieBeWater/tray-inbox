package main

import (
	"context"
	"errors"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
	"github.com/jinjiebewater/tray-inbox/internal/inbox"
)

// run starts the adapters, applies their events in arrival order, and publishes the view first
// and then whenever it changes. It returns after ctx is done and every adapter has returned.
func run(ctx context.Context, adapters []agent.Adapter, now func() time.Time, publish func(inbox.View)) {
	events := make(chan agent.Event)
	emit := func(ev agent.Event) {
		select {
		case events <- ev:
		case <-ctx.Done():
		}
	}
	var wg sync.WaitGroup
	for _, a := range adapters {
		wg.Go(func() {
			if err := a.Run(ctx, emit); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("%s: %v", a.Orchestrator(), err)
			}
		})
	}
	stopped := make(chan struct{})
	go func() {
		wg.Wait()
		close(stopped)
	}()

	model := inbox.New()
	last := model.View()
	publish(last)
	for {
		select {
		case ev := <-events:
			model.Apply(ev, now())
			if v := model.View(); !reflect.DeepEqual(v, last) {
				last = v
				publish(v)
			}
		case <-stopped:
			return
		}
	}
}

func jump(ctx context.Context, adapters []agent.Adapter, id agent.ID) error {
	for _, a := range adapters {
		if a.Orchestrator() == id.Orchestrator {
			return a.Jump(ctx, id)
		}
	}
	return errors.New("no adapter for " + id.Orchestrator)
}
