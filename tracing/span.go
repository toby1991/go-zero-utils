package tracing

import (
	"context"
	ztrace "github.com/zeromicro/go-zero/core/trace"
	"go.opentelemetry.io/otel/trace"
)

import "go.opentelemetry.io/otel/attribute"

func Span(ctx context.Context, name string, attr map[string]string) {
	var attrArr []attribute.KeyValue
	for k, v := range attr {
		attrArr = append(attrArr, attribute.String(k, v))
	}

	_, span := ztrace.TracerFromContext(ctx).Start(ctx, name, trace.WithAttributes(attrArr...))
	defer span.End()
}
