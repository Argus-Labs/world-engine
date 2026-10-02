package transport

import "go.opentelemetry.io/otel/attribute"

// Span names and attribute keys emitted by the transport. They keep their cardinal names: dashboards
// and alerts key on them.
const (
	spanEventPublish   = "cardinal.event.publish"
	spanInterShardSend = "cardinal.command.send"

	attrCommandName        = attribute.Key("cardinal.command.name")
	attrCommandTarget      = attribute.Key("cardinal.command.target")
	attrEventName          = attribute.Key("cardinal.event.name")
	attrEventRecipient     = attribute.Key("cardinal.event.recipient")
	attrEventSubscribers   = attribute.Key("cardinal.event.subscribers")
	attrEventWaiters       = attribute.Key("cardinal.event.waiters")
	attrEventSendFailures  = attribute.Key("cardinal.event.send_failures")
	attrEventSubscriptions = attribute.Key("cardinal.event.subscriptions")
)
