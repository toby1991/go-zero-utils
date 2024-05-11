package api

import (
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
		httpx.WriteJson(w, http.StatusUnprocessableEntity, &Body{
			Code: -1,
			Msg:  err.Error(),
			Data: nil,
		})
		return
	}

	httpx.OkJson(w, &Body{
		Code: 0,
		Msg:  "OK",
		Data: resp,
	})
	return
}
