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

	if !resp.Allowed {
		log.Error(nil, "admission request denied",
			"webhook", h.Name,
			"namespace", req.Namespace,
			"name", req.Name,
			"reason", resp.Result.Reason,
			"message", resp.Result.Message,
		)
	} else {
		log.Info("admission request allowed",
			"webhook", h.Name,
			"namespace", req.Namespace,
			"name", req.Name,
			"reason", resp.Result.Reason,
			"message", resp.Result.Message,
			"patchType", resp.PatchType,
		)
	}

	return resp
}
