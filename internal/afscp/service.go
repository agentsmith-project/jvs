package afscp

import (
	"context"
	"strings"
)

type Request struct {
	Selector    Selector
	Message     string
	SavePointID string
}

type SavePoint struct {
	SavePointID string `json:"save_point_id"`
	CreatedAt   string `json:"created_at,omitempty"`
	Message     string `json:"message,omitempty"`
	HistoryHead bool   `json:"history_head,omitempty"`
}

type SaveResult struct {
	SavePointID string `json:"save_point_id"`
	CreatedAt   string `json:"created_at"`
	Message     string `json:"message"`
	HistoryHead string `json:"history_head"`
}

type RestoreResult struct {
	RestoredSavePointID string  `json:"restored_save_point_id"`
	PreviousHead        *string `json:"previous_head"`
	NewHead             string  `json:"new_head"`
}

type ListResult struct {
	HistoryHead   *string     `json:"history_head"`
	SavePoints    []SavePoint `json:"save_points"`
	MetadataState string      `json:"metadata_state"`
}

type StatusResult struct {
	RepoID          string  `json:"repo_id"`
	HistoryHead     *string `json:"history_head"`
	ActiveOperation string  `json:"active_operation"`
	MetadataState   string  `json:"metadata_state"`
	Recovery        string  `json:"recovery"`
}

type DoctorResult struct {
	Healthy       bool                `json:"healthy"`
	RepoID        string              `json:"repo_id"`
	Findings      []FindingProjection `json:"findings"`
	MetadataState string              `json:"metadata_state"`
	Journal       string              `json:"journal"`
	Recovery      string              `json:"recovery"`
}

type FindingProjection struct {
	Code      ErrorCode `json:"code,omitempty"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
}

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Save(ctx context.Context, request Request) (any, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}
	selector, err := ValidateSelector(request.Selector)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.Message) == "" {
		return nil, NewError(ErrorCodeInvalidArgument, "save requires --message", false)
	}
	result, err := saveDirect(ctx, selector, strings.TrimSpace(request.Message))
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) List(ctx context.Context, request Request) (ListResult, error) {
	if err := validateContext(ctx); err != nil {
		return ListResult{}, err
	}
	selector, err := ValidateSelector(request.Selector)
	if err != nil {
		return ListResult{}, err
	}
	return listDirect(selector)
}

func (s *Service) Restore(ctx context.Context, request Request) (any, error) {
	if err := validateContext(ctx); err != nil {
		return nil, err
	}
	selector, err := ValidateSelector(request.Selector)
	if err != nil {
		return nil, err
	}
	if !validSavePointID(request.SavePointID) {
		return nil, NewError(ErrorCodeInvalidArgument, "restore requires a valid --save-point", false)
	}
	result, err := restoreDirect(ctx, selector, request.SavePointID)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) Status(ctx context.Context, request Request) (StatusResult, error) {
	if err := validateContext(ctx); err != nil {
		return StatusResult{}, err
	}
	selector, err := ValidateMetadataSelector(request.Selector)
	if err != nil {
		return StatusResult{}, err
	}
	return statusDirect(selector)
}

func (s *Service) Doctor(ctx context.Context, request Request) (DoctorResult, error) {
	if err := validateContext(ctx); err != nil {
		return DoctorResult{}, err
	}
	selector, err := ValidateMetadataSelector(request.Selector)
	if err != nil {
		return DoctorResult{}, err
	}
	return doctorDirect(selector)
}

func validateContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return NewError(ErrorCodeInternal, "operation canceled", false)
	}
	return nil
}

func validSavePointID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || id == "." || id == ".." {
		return false
	}
	return !strings.ContainsAny(id, `/\`) && !strings.Contains(id, "..")
}
