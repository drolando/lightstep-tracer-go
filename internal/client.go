package internal

import (
	"context"
)

type Client interface {
	Report(context.Context, ReportRequest) (ReportResponse, error)
	Close(context.Context) error
}

type ReportRequest struct {
	Spans []SpanRequest
}

type ReportResponse struct{}

type SpanRequest struct {
	OperationName string
}

type NoopClient struct{}

func (c NoopClient) Report(context.Context, ReportRequest) (ReportResponse, error) {
	return ReportResponse{}, nil
}

func (c NoopClient) Close(context.Context) error {
	return nil
}
