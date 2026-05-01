package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (s *Server) registerTranscodeRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID:   "cancelTranscodeJob",
		Method:        http.MethodPost,
		Path:          "/api/v1/transcode/cancel/{jobId}",
		Summary:       "Cancel a transcode job",
		Description:   "Cancels a pending or running transcode job. Returns 404 if the job does not exist, is already completed, or is already cancelled.",
		Tags:          []string{"Transcode"},
		Security:      []map[string][]string{{"bearer": {}}},
		DefaultStatus: http.StatusNoContent,
	}, s.handleCancelTranscode)
}

// CancelTranscodeInput contains parameters for cancelling a transcode job.
type CancelTranscodeInput struct {
	Authorization string `header:"Authorization"`
	JobID         string `path:"jobId" doc:"Transcode job ID"`
}

// CancelTranscodeOutput is the empty 204 response.
type CancelTranscodeOutput struct{}

func (s *Server) handleCancelTranscode(ctx context.Context, input *CancelTranscodeInput) (*CancelTranscodeOutput, error) {
	if _, err := GetUserID(ctx); err != nil {
		return nil, err
	}

	if err := s.services.Transcode.CancelJob(ctx, input.JobID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, huma.Error404NotFound("transcode job not found or not cancellable")
		}
		return nil, huma.Error500InternalServerError("failed to cancel transcode job: " + err.Error())
	}

	return &CancelTranscodeOutput{}, nil
}
