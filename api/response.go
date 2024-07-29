package api

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"
)

type Body struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

func Response(w http.ResponseWriter, resp interface{}, err error) {
	if err != nil {

		// grpc return process
		if grpcResponse(w, err) {
			return
		}

		// default
		httpx.WriteJson(w, http.StatusUnprocessableEntity, &Body{
			Code: -1,
			Msg:  err.Error(),
			Data: nil,
		})
		return
	}

	// success
	httpx.OkJson(w, &Body{
		Code: 0,
		Msg:  "OK",
		Data: resp,
	})
	return
}

// process grpc response, if processed, return true
func grpcResponse(w http.ResponseWriter, err error) bool {
	// no error
	if err == nil {
		return false
	}

	ev, ok := status.FromError(err)
	// not grpc error
	if !ok {
		return false
	}

	code := ev.Code()
	switch code {
	case codes.NotFound:
		httpx.WriteJson(w, http.StatusNotFound, &Body{
			Code: -1,
			Msg:  ev.Message(),
			Data: nil,
		})
		return true
	case codes.PermissionDenied:
		httpx.WriteJson(w, http.StatusForbidden, &Body{
			Code: -1,
			Msg:  ev.Message(),
			Data: nil,
		})
		return true
	default:
		// default
		httpx.WriteJson(w, http.StatusUnprocessableEntity, &Body{
			Code: -1,
			Msg:  ev.Message(),
			Data: nil,
		})
		return true
	}
}
