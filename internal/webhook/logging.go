package webhook

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// LoggingHandler wraps an admission.Handler and logs requests and responses.
type LoggingHandler struct {
	Handler admission.Handler
	Name    string
}

// Handle processes the admission request and logs the result.
func (h *LoggingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	log.Info("serving admission request",
		"webhook", h.Name,
		"kind", req.Kind,
		"namespace", req.Namespace,
		"name", req.Name,
		"operation", req.Operation,
		"uid", req.UID,
	)

	resp := h.Handler.Handle(ctx, req)

	var msg, reason string
	if resp.Result != nil {
		msg = resp.Result.Message
		reason = string(resp.Result.Reason)
	}
	if !resp.Allowed {
		log.Error(nil, "admission request denied",
			"webhook", h.Name,
			"namespace", req.Namespace,
			"name", req.Name,
			"reason", reason,
			"message", msg,
		)
	} else {
		var msg, reason string
		if resp.Result != nil {
			msg = resp.Result.Message
			reason = string(resp.Result.Reason)
		}
		log.Info("admission request allowed",
			"webhook", h.Name,
			"namespace", req.Namespace,
			"name", req.Name,
			"reason", reason,
			"message", msg,
			"patchType", resp.PatchType,
		)
	}

	return resp
}
