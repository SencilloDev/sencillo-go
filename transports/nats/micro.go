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
)

type HandlerWithErrors func(*slog.Logger, micro.Request) error
type AppHandler func(r micro.Request, h HandlerContext) error

type HandlerContext struct {
	Logger *slog.Logger
	Conn   *nats.Conn
}

type AppContext struct {
	Conn   *nats.Conn
	Logger *slog.Logger
}

type ClientError interface {
	Error() string
	Code() int
	Body() []byte
	LoggedError() []error
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
func ErrorHandler(a AppContext, handler AppHandler) micro.HandlerFunc {
	return func(r micro.Request) {
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
		ctx := HandlerContext{
			Logger: reqLogger,
			Conn:   a.Conn,
		}

		err = handler(r, ctx)
		if err == nil {
			return
		}

		handleRequestError(reqLogger, err, r)
	}
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
