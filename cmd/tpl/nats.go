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

package tpl

func Nats() []byte {
	return []byte(`{{ $tick := "` + "`" + `" -}}
package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	sderrors "github.com/SencilloDev/sencillo-go/errors"
	sdnats "github.com/SencilloDev/sencillo-go/transports/nats"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/micro"
)

type Handler func(micro.Request, CustomCtx) error

type CustomCtx struct {
	HandlerCtx sdnats.HandlerContext
	Something  string
}

type MathRequest struct {
	A int {{ $tick }}json:"a"{{ $tick }}
	B int {{ $tick }}json:"b"{{ $tick }}
}

type MathResponse struct {
	Result int {{ $tick }}json:"result"{{ $tick }}
}

func Wrapper(handler Handler, custom CustomCtx) sdnats.AppHandler {
	return func(r micro.Request, h sdnats.HandlerContext) error {
		custom.HandlerCtx = h
		return handler(r, custom)
	}
}

func SpecificHandler(r micro.Request, c CustomCtx) error {
	fmt.Println(c.Something)
	msg := sdnats.RequestToMsg(r)
	msg.Subject = "testing.things"
	resp, err := c.HandlerCtx.Conn.RequestMsg(msg, 1*time.Second)
	if err != nil {
		return err
	}

	return r.Respond(resp.Data)
}

func Add(r micro.Request, h sdnats.HandlerContext) error {
	var mr MathRequest
	if err := json.Unmarshal(r.Data(), &mr); err != nil {
		return sderrors.NewClientError(err, 400)
	}

	resp := MathResponse{Result: mr.A + mr.B}

	return r.RespondJSON(resp)
}

func Subtract(r micro.Request, h sdnats.HandlerContext) error {
	var mr MathRequest
	if err := json.Unmarshal(r.Data(), &mr); err != nil {
		return sderrors.NewClientError(err, 400)
	}

	resp := MathResponse{Result: mr.A - mr.B}

	return r.RespondJSON(resp)
}

func WatchForConfig(logger *slog.LevelVar, js nats.JetStreamContext) {
	kv, err := js.KeyValue("configs")
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	w, err := kv.Watch("{{ .Name }}.log_level")
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}

	for val := range w.Updates() {
		if val == nil {
			continue
		}

		level := string(val.Value())
		if level == "info" {
			slog.SetLogLoggerLevel(slog.LevelInfo)	
		}

		if level == "error" {
			slog.SetLogLoggerLevel(slog.LevelError)	
		}

		if level == "debug" {
			slog.SetLogLoggerLevel(slog.LevelDebug)	
		}

		slog.Info(fmt.Sprintf("set log level to %s", level))
	}

	time.Sleep(5 * time.Second)
}
`)
}
