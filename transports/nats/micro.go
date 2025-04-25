// Copyright 2025 Sencillo
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nats

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	sderrors "github.com/SencilloDev/sencillo-go/errors"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
	"github.com/segmentio/ksuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type HandlerWithErrors func(*slog.Logger, micro.Request) error
type AppHandler func(ctx context.Context, r micro.Request, h HandlerContext) error
type microHeaderCarrier micro.Headers

type HandlerContext struct {
	Logger     *slog.Logger
	Conn       *nats.Conn
	Tracer     trace.Tracer
	Propagator propagation.TextMapPropagator
}

type AppContext struct {
	Conn       *nats.Conn
	Logger     *slog.Logger
	Tracer     trace.Tracer
	Propagator propagation.TextMapPropagator
}

type ClientError interface {
	Error() string
	Code() int
	Body() []byte
	LoggedError() []error
}

func (m microHeaderCarrier) Get(key string) string {
	return micro.Headers(m).Get(key)
}

func (m microHeaderCarrier) Set(key, val string) {
	m[key] = []string{val}
}

func (m microHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(micro.Headers(m)))
	for k := range nats.Header(m) {
		keys = append(keys, k)
	}
	return keys
}

func (h HandlerContext) InjectTraceHeaders(ctx context.Context, headers map[string][]string) {
	h.Propagator.Inject(ctx, microHeaderCarrier(headers))
}

func InjectTraceHeaders(ctx context.Context, p propagation.TextMapPropagator, headers map[string][]string) {
	p.Inject(ctx, microHeaderCarrier(headers))
}

func HandleNotify(s micro.Service, healthFuncs ...func(chan<- string, micro.Service)) error {
	stopChan := make(chan string, 1)
	for _, v := range healthFuncs {
		go v(stopChan, s)
	}

	go handleNotify(stopChan)

	slog.Info(<-stopChan)
	return s.Stop()
}

func handleNotify(stopChan chan<- string) {
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigTerm
	stopChan <- fmt.Sprintf("received signal: %v", sig)
}

// ErrorHandler wraps a normal micro endpoint and allows for returning errors natively. Errors are
// checked and if an error is a client error, details are returned, otherwise a 500 is returned and logged
func ErrorHandler(name string, a AppContext, handler AppHandler) micro.Handler {
	ctx := context.Background()
	return micro.ContextHandler(ctx, func(ctx context.Context, r micro.Request) {
		start := time.Now()
		id, err := MsgID(r)
		if err != nil {
			handleRequestError(a.Logger, sderrors.NewClientError(err, 400), r)
			return
		}
		reqLogger := a.Logger.With("request_id", id, "path", r.Subject())
		defer func() {
			reqLogger.Info(fmt.Sprintf("duration %dms", time.Since(start).Milliseconds()))
		}()

		if err := buildQueryHeaders(r); err != nil {
			handleRequestError(reqLogger, err, r)
		}
		handlerCtx := HandlerContext{
			Logger:     reqLogger,
			Conn:       a.Conn,
			Tracer:     a.Tracer,
			Propagator: a.Propagator,
		}

		headers := r.Headers()
		newCtx := a.Propagator.Extract(ctx, microHeaderCarrier(headers))
		startCtx, span := a.Tracer.Start(newCtx, name)
		span.SetAttributes(attribute.KeyValue{Key: "X-Request-ID", Value: attribute.StringValue(id)})
		defer span.End()

		err = handler(startCtx, r, handlerCtx)
		if err == nil {
			span.SetStatus(codes.Ok, "success")
			return
		}

		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)

		handleRequestError(reqLogger, err, r)

	})
}

// Create Sencillo specific headers from the NATS bridge plugin headers
func buildQueryHeaders(r micro.Request) error {
	headers := nats.Header(r.Headers())
	query := headers.Get("X-NatsBridge-UrlQuery")
	parsed, err := url.ParseQuery(query)
	if err != nil {
		return err
	}

	for k, v := range parsed {
		key := fmt.Sprintf("X-Sencillo-%s", k)
		headers[key] = v
	}

	return nil
}

func GetQueryHeaders(headers micro.Headers, key string) []string {
	k := fmt.Sprintf("X-Sencillo-%s", key)
	return headers.Values(k)
}

func handleRequestError(logger *slog.Logger, err error, r micro.Request) {
	ce, ok := err.(ClientError)
	if ok {
		for _, v := range ce.LoggedError() {
			logger.Error(v.Error())
		}
		r.Error(fmt.Sprintf("%d", ce.Code()), http.StatusText(ce.Code()), ce.Body())
		return
	}

	logger.Error(err.Error())

	r.Error("500", "internal server error", []byte(`{"errors": ["internal server error"]}`))
}

func MsgID(r micro.Request) (string, error) {
	id := r.Headers().Get("X-Request-ID")
	if id == "" {
		return "", fmt.Errorf("required request ID not found")
	}

	return id, nil
}

func RequestLogger(l *slog.Logger, r micro.Request) (*slog.Logger, error) {
	id, err := MsgID(r)
	if err != nil {
		return nil, err
	}
	return l.With("request_id", id), nil
}

func NewMsgWithID() *nats.Msg {
	headers := map[string][]string{
		"X-Request-ID": {ksuid.New().String()},
	}
	return &nats.Msg{
		Header: headers,
	}
}

func RequestToMsg(r micro.Request) *nats.Msg {
	return &nats.Msg{
		Header: nats.Header(r.Headers()),
		Data:   r.Data(),
	}
}
