package incus

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"path"
	"strings"

	incusapi "github.com/lxc/incus/v7/shared/api"

	"github.com/FuturFusion/operations-center/internal/domain"
	"github.com/FuturFusion/operations-center/internal/provisioning"
	"github.com/FuturFusion/operations-center/internal/util/logger"
	"github.com/FuturFusion/operations-center/internal/util/ptr"
)

func (c Client) SubscribeLifecycleEvents(ctx context.Context, endpoint provisioning.Endpoint) (chan domain.LifecycleEvent, chan error, error) {
	client, err := c.getClient(ctx, endpoint)
	if err != nil {
		return nil, nil, err
	}

	listener, err := client.GetEventsAllProjectsByType([]string{incusapi.EventTypeLifecycle})
	if err != nil {
		return nil, nil, err
	}

	// Allow for up to 100 in-flight events to prevent the sender or the websocket
	// connection from being blocked due to slow processing.
	lifecycleEvents := make(chan domain.LifecycleEvent, 100)
	errChan := make(chan error)
	stopped := make(chan struct{})
	// ignore the error, only happens, if the passed function is nil.
	target, _ := listener.AddHandler([]string{incusapi.EventTypeLifecycle}, func(event incusapi.Event) {
		lifecycleEvent, ok, err := mapIncusEventToLifecycleEvent(ctx, event)
		if err != nil {
			slog.WarnContext(ctx, "Failed to map incus event to lifecycle event", logger.Err(err))
			return
		}

		if !ok {
			return
		}

		select {
		case lifecycleEvents <- lifecycleEvent:
		case <-stopped:
		case <-ctx.Done():
		}
	})

	go func() {
		select {
		// Disconnect, if we are done and the context is cancelled.
		case <-ctx.Done():
			// ignore the error, unlikely to happen und there is not really anything we can do about it.
			_ = listener.RemoveHandler(target)

			listener.Disconnect()

		// Signal, if listener disconnect.
		case errChan <- listener.Wait():
			// ignore the error, unlikely to happen und there is not really anything we can do about it.
			_ = listener.RemoveHandler(target)
		}

		// Stop the senders. Do not close lifecycleEvents. Handlers, that have been
		// spawned before the handler has been removed, might still send on it.
		close(stopped)
		close(errChan)
	}()

	return lifecycleEvents, errChan, nil
}

func mapIncusEventToLifecycleEvent(ctx context.Context, event incusapi.Event) (domain.LifecycleEvent, bool, error) {
	if event.Type != incusapi.EventTypeLifecycle {
		return domain.LifecycleEvent{}, false, nil
	}

	incusLifecycleEvent := incusapi.EventLifecycle{}
	err := json.Unmarshal(event.Metadata, &incusLifecycleEvent)
	if err != nil {
		return domain.LifecycleEvent{}, false, err
	}

	slog.DebugContext(
		ctx, "map incus event to lifecycle event - inputs",
		slog.Any("event", map[string]any{
			"type":     event.Type,
			"project":  event.Project,
			"location": event.Location,
		}),
		slog.Any("metadata", map[string]any{
			"action":    incusLifecycleEvent.Action,
			"context":   incusLifecycleEvent.Context,
			"name":      incusLifecycleEvent.Name,
			"source":    incusLifecycleEvent.Source,
			"project":   incusLifecycleEvent.Project,
			"requestor": ptr.From(incusLifecycleEvent.Requestor),
		}),
	)

	lifecycleResourceTypeOperation, ok := domain.MapLifecycleAction[incusLifecycleEvent.Action]
	if !ok {
		return domain.LifecycleEvent{}, false, nil
	}

	// Example values of incusLifecycleEvent.Source:
	//
	//   /1.0/instances/d1 -> no parent
	//   /1.0/storage-pools/default/volumes/custom/default_foo -> parent type: storage-pools, parent name: default
	//   /1.0/profiles/some-profile?project=some-project
	sourceURL, err := url.Parse(incusLifecycleEvent.Source)
	if err != nil {
		return domain.LifecycleEvent{}, false, err
	}

	name := firstNonEmpty(incusLifecycleEvent.Name, path.Base(sourceURL.Path))
	projectName := firstNonEmpty(incusLifecycleEvent.Project, event.Project, sourceURL.Query().Get("project"), "default")

	// Process source for the existens of a parent.
	var lifecycleEventParentType string
	var lifecycleEventParentName string
	sourceParts := strings.Split(strings.TrimLeft(sourceURL.Path, "/"), "/")
	if len(sourceParts) > 3 {
		lifecycleEventParentType, _ = strings.CutSuffix(sourceParts[1], "s") // remove pluralization
		lifecycleEventParentName = sourceParts[2]
	}

	// Rename events provide the old name of the resource in "old_name".
	var oldName string
	if lifecycleResourceTypeOperation.Operation == domain.LifecycleOperationRename {
		oldNameAny, ok := incusLifecycleEvent.Context["old_name"]
		if ok {
			oldName, _ = oldNameAny.(string) // nolint:revive // zero value is ok, if the type assertion fails.
		}
	}

	// For storage volumes, the type is also part of the identifying key.
	var lifecycleEventType string
	if lifecycleResourceTypeOperation.ResourceType == domain.ResourceTypeStorageVolume {
		incusEventContextTypeAny, ok := incusLifecycleEvent.Context["type"]
		if ok {
			lifecycleEventType, _ = incusEventContextTypeAny.(string) // nolint:revive // zero value is ok, if the type assertion fails.
		}

		lifecycleEventType = firstNonEmpty(
			lifecycleEventType,
			path.Base(path.Dir(sourceURL.Path)), // Second last segment of the path is the type of the storage volume.
		)
	}

	ret := domain.LifecycleEvent{
		LifecycleEventAction: incusLifecycleEvent.Action,
		ResourceType:         lifecycleResourceTypeOperation.ResourceType,
		Operation:            lifecycleResourceTypeOperation.Operation,
		Source: domain.LifecycleSource{
			ParentType:  lifecycleEventParentType,
			ParentName:  lifecycleEventParentName,
			ProjectName: projectName,
			Name:        name,
			Type:        lifecycleEventType,
			OldName:     oldName,
		},
	}

	slog.DebugContext(
		ctx, "map incus event to lifecycle event - return",
		slog.Any("lifecycle_event", ret),
	)

	return ret, true, nil
}

func firstNonEmpty(candidates ...string) string {
	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}

	return ""
}
