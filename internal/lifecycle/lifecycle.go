package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
)

// Container is a collection of components which run in separate
// goroutines but must be shut-down in order.
type Container struct {
	Components []func(ctx context.Context) error
}

func (a *Container) Run(ctx context.Context) error {
	type threadInfo struct {
		cancel func(error)
		done   <-chan struct{}
		err    func() error
	}
	var threads []threadInfo

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	midContext := context.WithoutCancel(ctx)

	var wg sync.WaitGroup

	// start components
	for _, fn := range a.Components {
		threadCtx, threadCancel := context.WithCancelCause(midContext)
		threadDone := make(chan struct{})
		wg.Go(func() {
			// start a shutdown as soon as any thread is done
			defer cancel()
			// mark this thread as done
			defer close(threadDone)
			// run!
			threadCancel(fn(threadCtx))
		})
		threads = append(threads, threadInfo{
			cancel: threadCancel,
			done:   threadDone,
			err: func() error {
				err := context.Cause(threadCtx)
				if err != threadCtx.Err() {
					return err
				}
				return nil
			},
		})
	}

	slog.InfoContext(ctx, "launched all components")
	<-ctx.Done()

	slog.InfoContext(ctx, "shutting down components")
	var errs []error

	// shutdown components
	slices.Reverse(threads)
	for i := range threads {
		threadInfo := &threads[i]
		threadInfo.cancel(nil)
		// TODO - timeout?
		<-threadInfo.done
		errs = append(errs, threadInfo.err())
	}

	// shouldn't be needed
	wg.Wait()

	slog.InfoContext(ctx, "completed shutdown")

	return errors.Join(errs...)
}
