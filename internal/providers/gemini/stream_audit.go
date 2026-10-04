package gemini

import (
	"context"
	"errors"
	"io"
	"net/http"
)

// genai logs scanner failures instead of yielding them. Successful iterator
// exhaustion therefore requires both a native terminal and clean body EOF.
type streamAuditKey struct{}
type streamAudit struct {
	eof     chan struct{}
	failure chan error
}

func auditedStreamContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, streamAuditKey{}, &streamAudit{
		eof: make(chan struct{}, 1), failure: make(chan error, 1),
	})
}

type streamAuditTransport struct{ base http.RoundTripper }

func (transport streamAuditTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	audit, ok := request.Context().Value(streamAuditKey{}).(*streamAudit)
	if !ok || response.Body == nil {
		return response, nil
	}
	clone := *response
	clone.Body = streamAuditBody{ReadCloser: response.Body, audit: audit}
	return &clone, nil
}

type streamAuditBody struct {
	io.ReadCloser
	audit *streamAudit
}

func (body streamAuditBody) Read(buffer []byte) (int, error) {
	n, err := body.ReadCloser.Read(buffer)
	if errors.Is(err, io.EOF) {
		select {
		case body.audit.eof <- struct{}{}:
		default:
		}
	} else if err != nil {
		select {
		case body.audit.failure <- err:
		default:
		}
	}
	return n, err
}

func streamReadFailure(ctx context.Context) error {
	audit, ok := ctx.Value(streamAuditKey{}).(*streamAudit)
	if !ok {
		return invalidGeminiToolResponse()
	}
	select {
	case err := <-audit.failure:
		return errors.Join(invalidGeminiToolResponse(), err)
	default:
	}
	select {
	case <-audit.eof:
		return nil
	default:
		return invalidGeminiToolResponse()
	}
}
