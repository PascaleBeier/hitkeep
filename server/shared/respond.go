package shared

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	json "hitkeep/jsonapi"
)

// WriteJSON encodes value with the jsonapi wire policy before committing the
// status, so an encoding failure (for example invalid UTF-8, which the v2
// encoder rejects) becomes a 500 instead of a truncated 2xx body. Streaming
// responses such as exports should keep using json.MarshalWrite directly.
func WriteJSON(ctx context.Context, w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		LoggerFromContext(ctx).Error("Failed to encode response", "error", err, "type", fmt.Sprintf("%T", value))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "application/json")
	header.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// PathUUID parses the named path value as a UUID. On failure it writes a 400
// with message and returns false.
func PathUUID(w http.ResponseWriter, r *http.Request, name, message string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue(name)))
	if err != nil {
		http.Error(w, message, http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}
